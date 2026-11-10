package cutline

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAPI serves the two GitHub endpoints cutline uses.
func fakeAPI(t *testing.T, status int) (*GitHub, struct{}) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/app/pulls/7", func(w http.ResponseWriter, req *http.Request) {
		if status != 200 {
			w.WriteHeader(status)
			return
		}
		if req.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"number":7,"title":"Add the parser","html_url":"https://example.test/pull/7","user":{"login":"octo"},"labels":[{"name":"bug"},{"name":"p1"}]}`))
	})
	mux.HandleFunc("/repos/acme/app/commits/", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/pulls") {
			_, _ = w.Write([]byte(`[{"number":9,"title":"Open one","merged_at":null,"user":{"login":"x"}},{"number":8,"title":"Merged one","merged_at":"2026-10-01T00:00:00Z","user":{"login":"y"},"labels":[]}]`))
			return
		}
		w.WriteHeader(404)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &GitHub{Owner: "acme", Repo: "app", Token: "tok", BaseURL: srv.URL}, struct{}{}
}

func TestGitHubPullRequest(t *testing.T) {
	g, _ := fakeAPI(t, 200)
	p, err := g.PullRequest(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Add the parser" || p.Author != "octo" || len(p.Labels) != 2 || p.URL == "" {
		t.Errorf("PRInfo = %+v", p)
	}
	if _, err := g.PullRequest(context.Background(), 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestGitHubCommitPullRequestPicksMerged(t *testing.T) {
	g, _ := fakeAPI(t, 200)
	p, err := g.CommitPullRequest(context.Background(), "abc123")
	if err != nil || p.Number != 8 {
		t.Fatalf("p = %+v, err = %v", p, err)
	}
}

func TestGitHubStatusErrors(t *testing.T) {
	for _, code := range []int{401, 403, 429} {
		g, _ := fakeAPI(t, code)
		if _, err := g.PullRequest(context.Background(), 7); !errors.Is(err, ErrAuth) {
			t.Errorf("status %d: err = %v, want ErrAuth", code, err)
		}
	}
	g, _ := fakeAPI(t, 500)
	if _, err := g.PullRequest(context.Background(), 7); err == nil || errors.Is(err, ErrAuth) || errors.Is(err, ErrNotFound) {
		t.Errorf("500: err = %v", err)
	}
	if _, err := (&GitHub{}).PullRequest(context.Background(), 1); err == nil {
		t.Error("empty owner and repo should fail")
	}
}

func TestDiffEnrichesEntries(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("7", "parser", "wip")
	r.rebasePR("loose commit")
	g, _ := fakeAPI(t, 200)
	res := r.diff(Options{Enricher: g})
	if len(res.OnlyA) != 2 {
		t.Fatalf("OnlyA = %+v", res.OnlyA)
	}
	byPR := map[int]Entry{}
	for _, e := range res.OnlyA {
		byPR[e.PR] = e
	}
	e7, ok := byPR[7]
	if !ok || e7.Title != "Add the parser" || e7.Author != "octo" || e7.URL == "" || len(e7.Labels) != 2 {
		t.Errorf("entry 7 = %+v", e7)
	}
	e8, ok := byPR[8]
	if !ok || e8.Title != "Merged one" || e8.Note != "" {
		t.Errorf("single commit not resolved to its pull request: %+v", res.OnlyA)
	}
	if len(res.Notices) != 0 {
		t.Errorf("Notices = %v", res.Notices)
	}
}

func TestDiffEnrichmentFailureIsANotice(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("7", "parser", "wip")
	g, _ := fakeAPI(t, 403)
	res := r.diff(Options{Enricher: g})
	if len(res.OnlyA) != 1 || res.OnlyA[0].Title != "parser" {
		t.Errorf("git data should stay: %+v", res.OnlyA)
	}
	if len(res.Notices) != 1 || !strings.Contains(res.Notices[0], "github") {
		t.Errorf("Notices = %v", res.Notices)
	}
	var sb strings.Builder
	if err := res.Write(&sb, FormatText); err != nil || !strings.Contains(sb.String(), "note: github") {
		t.Errorf("text output lacks the note: %q, %v", sb.String(), err)
	}
}

func TestNotesGroupByLabel(t *testing.T) {
	r := newRepo(t)
	r.on("stage")
	r.squashPR("7", "parser", "wip")
	g, _ := fakeAPI(t, 200)
	n, err := BuildNotes(context.Background(), "stage", "prod", PlanOptions{Options: Options{Dir: r.dir, Enricher: g}})
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Sections) != 1 || n.Sections[0].Name != "Fixes" {
		t.Errorf("Sections = %+v", n.Sections)
	}
}

func TestSectionFromLabels(t *testing.T) {
	if got := sectionFromLabels([]string{"p1", " Enhancement "}); got != "Features" {
		t.Errorf("got %q", got)
	}
	if got := sectionFromLabels(nil); got != "Other" {
		t.Errorf("got %q", got)
	}
}
