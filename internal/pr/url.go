package pr

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// PRInfo holds the parsed components of a GitHub pull request URL.
type PRInfo struct {
	Owner      string
	Repo       string
	PullNumber int
}

// ParsePRURL parses a GitHub PR URL and extracts owner, repo, and pull number.
// Supported formats:
//
//	https://github.com/owner/repo/pull/123
//	https://github.com/owner/repo/pull/123/files
//	http://github.com/owner/repo/pull/123
func ParsePRURL(rawURL string) (*PRInfo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("PR URL is empty")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("unsupported URL scheme %q: must be https or http", u.Scheme)
	}

	if u.Host != "github.com" {
		return nil, fmt.Errorf("unsupported host %q: only github.com PR URLs are supported", u.Host)
	}

	// Path segments: /owner/repo/pull/number[/...]
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid GitHub PR URL path: expected /owner/repo/pull/number")
	}

	if parts[2] != "pull" {
		return nil, fmt.Errorf("URL path does not contain /pull/: expected /owner/repo/pull/number")
	}

	owner := parts[0]
	repo := parts[1]
	pullStr := parts[3]

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner or repo is empty in URL path")
	}

	pullNumber, err := strconv.Atoi(pullStr)
	if err != nil {
		return nil, fmt.Errorf("invalid pull number %q: must be an integer", pullStr)
	}
	if pullNumber <= 0 {
		return nil, fmt.Errorf("pull number must be positive, got %d", pullNumber)
	}

	return &PRInfo{
		Owner:      owner,
		Repo:       repo,
		PullNumber: pullNumber,
	}, nil
}
