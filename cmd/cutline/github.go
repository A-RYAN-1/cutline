package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/A-RYAN-1/cutline"
)

// remoteRepo matches the owner and repository in an https or ssh GitHub URL.
var remoteRepo = regexp.MustCompile(`[:/]([^/:]+)/([^/]+?)(?:\.git)?/?$`)

// enricher builds the optional GitHub enricher from the -github flag. "auto"
// reads the origin remote. An empty value keeps cutline offline.
func (c *common) enricher() (cutline.Enricher, error) {
	if c.github == "" {
		return nil, nil
	}
	repo := c.github
	if repo == "auto" {
		cmd := exec.Command("git", "remote", "get-url", "origin")
		cmd.Dir = c.dir
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("-github auto: cannot read the origin remote: %w", err)
		}
		m := remoteRepo.FindStringSubmatch(strings.TrimSpace(string(out)))
		if m == nil {
			return nil, fmt.Errorf("-github auto: cannot read owner/repo from %q", strings.TrimSpace(string(out)))
		}
		repo = m[1] + "/" + m[2]
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("-github wants owner/repo or auto, got %q", c.github)
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return &cutline.GitHub{Owner: owner, Repo: name, Token: token, BaseURL: c.githubAPI}, nil
}
