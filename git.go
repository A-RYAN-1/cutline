package cutline

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func runGit(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// loadRange lists the commits reachable from include but not from exclude,
// newest first, with their patch ids.
func loadRange(ctx context.Context, dir, exclude, include, since string) ([]*Commit, error) {
	rng := exclude + ".." + include
	args := []string{"log", "--format=%H%x1f%P%x1f%an%x1f%aI%x1f%s%x1f%b%x1e"}
	if since != "" {
		args = append(args, "--since="+since)
	}
	out, err := runGit(ctx, dir, nil, append(args, rng, "--")...)
	if err != nil {
		return nil, err
	}
	var commits []*Commit
	for _, rec := range strings.Split(string(out), "\x1e") {
		rec = strings.TrimLeft(rec, "\r\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		f := strings.SplitN(rec, "\x1f", 6)
		if len(f) < 6 {
			return nil, fmt.Errorf("unexpected git log record %q", rec)
		}
		commits = append(commits, &Commit{
			SHA:     f[0],
			Parents: strings.Fields(f[1]),
			Author:  f[2],
			Date:    f[3],
			Subject: f[4],
			Body:    f[5],
			pr:      PRNumber(f[4]),
		})
	}
	if err := attachPatchIDs(ctx, dir, rng, since, commits); err != nil {
		return nil, err
	}
	return commits, nil
}

func attachPatchIDs(ctx context.Context, dir, rng, since string, commits []*Commit) error {
	if len(commits) == 0 {
		return nil
	}
	args := []string{"log", "-p", "--no-merges", "--no-color"}
	if since != "" {
		args = append(args, "--since="+since)
	}
	patch, err := runGit(ctx, dir, nil, append(args, rng, "--")...)
	if err != nil {
		return err
	}
	ids, err := runGit(ctx, dir, patch, "patch-id", "--stable")
	if err != nil {
		return err
	}
	bySHA := make(map[string]*Commit, len(commits))
	for _, c := range commits {
		bySHA[c.SHA] = c
	}
	for _, line := range strings.Split(string(ids), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 {
			if c := bySHA[f[1]]; c != nil {
				c.PatchID = f[0]
			}
		}
	}
	return nil
}

// mergeMembers lists the commits a merge commit brought in (first parent
// excluded).
func mergeMembers(ctx context.Context, dir, sha string) ([]string, error) {
	out, err := runGit(ctx, dir, nil, "rev-list", sha+"^1.."+sha+"^2", "--")
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}
