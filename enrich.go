package cutline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// PRInfo is what a hosting service knows about a pull request.
type PRInfo struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Labels []string `json:"labels,omitempty"`
	URL    string   `json:"url,omitempty"`
}

// Enricher looks up pull request data. It is optional: cutline works from git
// alone, and an Enricher only adds titles, authors, labels and links.
type Enricher interface {
	// PullRequest returns the pull request with the given number. It returns
	// ErrNotFound when there is none.
	PullRequest(ctx context.Context, number int) (PRInfo, error)
	// CommitPullRequest returns the pull request a commit was merged through,
	// or ErrNotFound when the commit belongs to none.
	CommitPullRequest(ctx context.Context, sha string) (PRInfo, error)
}

// ErrNotFound is returned by an Enricher when the pull request does not exist.
var ErrNotFound = errors.New("not found")

// ErrAuth is returned when the service refuses the request: a missing or bad
// token, or an exhausted rate limit. Enrichment stops at the first one.
var ErrAuth = errors.New("access refused or rate limit reached")

// maxLookups caps concurrent requests to the service.
const maxLookups = 8

// enrich fills Title, Author, Labels and URL of every entry from e, and finds
// the pull request of single commits that carry no number in their subject. A
// failure never fails the diff: it is recorded in res.Notices.
func enrich(ctx context.Context, res *Result, e Enricher) {
	if e == nil {
		return
	}
	groups := [][]Entry{res.OnlyA, res.OnlyB, res.Both, res.RevertedA, res.RevertedB}
	var mu sync.Mutex
	var warnOnce sync.Once
	stopped := false
	warn := func(msg string) {
		mu.Lock()
		res.Notices = append(res.Notices, msg)
		mu.Unlock()
	}
	sem := make(chan struct{}, maxLookups)
	var wg sync.WaitGroup
	for _, list := range groups {
		for i := range list {
			ent := &list[i]
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				mu.Lock()
				halt := stopped
				mu.Unlock()
				if halt {
					return
				}
				var info PRInfo
				var err error
				switch {
				case ent.PR != 0:
					info, err = e.PullRequest(ctx, ent.PR)
				case len(ent.Commits) > 0:
					info, err = e.CommitPullRequest(ctx, ent.Commits[0].SHA)
				default:
					return
				}
				switch {
				case err == nil:
					ent.apply(info)
				case errors.Is(err, ErrNotFound):
				case errors.Is(err, ErrAuth):
					mu.Lock()
					stopped = true
					mu.Unlock()
					warnOnce.Do(func() { warn("github: " + err.Error() + "; titles, authors and labels were not fetched for the rest") })
				default:
					warn("github: " + err.Error())
				}
			}()
		}
	}
	wg.Wait()
}

// apply copies hosting-service data onto an entry. The pull request title
// replaces the commit subject; a single commit found to belong to a pull
// request takes its number and loses the "no number" note.
func (e *Entry) apply(p PRInfo) {
	if e.PR == 0 && p.Number != 0 {
		e.PR = p.Number
		e.Note = ""
	}
	if p.Title != "" {
		e.Title = p.Title
	}
	if p.Author != "" {
		e.Author = p.Author
	}
	e.Labels = p.Labels
	e.URL = p.URL
}

// GitHub is an Enricher backed by the GitHub REST API. The zero value is not
// usable; set Owner and Repo.
type GitHub struct {
	Owner, Repo string
	// Token is a personal access token. Public repositories work without one
	// but share a low rate limit.
	Token string
	// BaseURL is the API root. Empty means https://api.github.com; set it for
	// GitHub Enterprise Server.
	BaseURL string
	// Client is the HTTP client. Nil means one with a 20 second timeout.
	Client *http.Client
}

type ghPull struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (p ghPull) info() PRInfo {
	out := PRInfo{Number: p.Number, Title: p.Title, Author: p.User.Login, URL: p.HTMLURL}
	for _, l := range p.Labels {
		out.Labels = append(out.Labels, l.Name)
	}
	return out
}

// PullRequest implements Enricher.
func (g *GitHub) PullRequest(ctx context.Context, number int) (PRInfo, error) {
	var p ghPull
	if err := g.get(ctx, fmt.Sprintf("pulls/%d", number), &p); err != nil {
		return PRInfo{}, err
	}
	return p.info(), nil
}

// CommitPullRequest implements Enricher. When a commit is in several pull
// requests it returns the first merged one, which is the one that brought it in.
func (g *GitHub) CommitPullRequest(ctx context.Context, sha string) (PRInfo, error) {
	var ps []struct {
		ghPull
		MergedAt *string `json:"merged_at"`
	}
	if err := g.get(ctx, "commits/"+url.PathEscape(sha)+"/pulls", &ps); err != nil {
		return PRInfo{}, err
	}
	for _, p := range ps {
		if p.MergedAt != nil {
			return p.info(), nil
		}
	}
	return PRInfo{}, ErrNotFound
}

func (g *GitHub) get(ctx context.Context, path string, into any) error {
	if g.Owner == "" || g.Repo == "" {
		return errors.New("github: owner and repo are required")
	}
	base := strings.TrimRight(g.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	u := fmt.Sprintf("%s/repos/%s/%s/%s", base, url.PathEscape(g.Owner), url.PathEscape(g.Repo), path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "cutline")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	c := g.Client
	if c == nil {
		c = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w (HTTP %d)", ErrAuth, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(into)
}
