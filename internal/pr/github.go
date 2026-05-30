package pr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	githubAPIBase   = "https://api.github.com"
	defaultPageSize = 100
)

// GitHubClient fetches PR data from the GitHub API.
type GitHubClient struct {
	Token      string
	HTTPClient *http.Client
	BaseURL    string
}

// NewGitHubClient creates a new GitHubClient with the given token.
func NewGitHubClient(token string) *GitHubClient {
	return &GitHubClient{
		Token: token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		BaseURL: githubAPIBase,
	}
}

// GetPRDetails fetches PR metadata from GET /repos/{owner}/{repo}/pulls/{pull_number}.
func (c *GitHubClient) GetPRDetails(ctx context.Context, owner, repo string, pullNumber int) (*PRDetails, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.BaseURL, owner, repo, pullNumber)
	resp, err := c.doRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp, url)
	}

	var details PRDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode PR details: %w", err)
	}
	return &details, nil
}

// GetChangedFiles fetches all changed files from GET /repos/{owner}/{repo}/pulls/{pull_number}/files.
// It handles pagination to return the complete file list.
func (c *GitHubClient) GetChangedFiles(ctx context.Context, owner, repo string, pullNumber int) ([]ChangedFile, error) {
	var allFiles []ChangedFile
	page := 1

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files?per_page=%d&page=%d",
			c.BaseURL, owner, repo, pullNumber, defaultPageSize, page)

		resp, err := c.doRequest(ctx, http.MethodGet, url)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, c.parseError(resp, url)
		}

		var files []ChangedFile
		if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode changed files: %w", err)
		}
		resp.Body.Close()

		allFiles = append(allFiles, files...)

		// Check if there are more pages via Link header.
		if !c.hasNextPage(resp) {
			break
		}
		page++
	}

	return allFiles, nil
}

// FetchPR fetches both PR details and changed files in a single call.
func (c *GitHubClient) FetchPR(ctx context.Context, owner, repo string, pullNumber int) (*PRData, error) {
	details, err := c.GetPRDetails(ctx, owner, repo, pullNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR details: %w", err)
	}

	files, err := c.GetChangedFiles(ctx, owner, repo, pullNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch changed files: %w", err)
	}

	return &PRData{
		Info: &PRInfo{
			Owner:      owner,
			Repo:       repo,
			PullNumber: pullNumber,
		},
		Details: details,
		Files:   files,
	}, nil
}

// doRequest performs an HTTP request with standard headers and auth.
func (c *GitHubClient) doRequest(ctx context.Context, method, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "ai-pr-review")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

// hasNextPage checks the Link header for a "next" relation.
func (c *GitHubClient) hasNextPage(resp *http.Response) bool {
	link := resp.Header.Get("Link")
	if link == "" {
		return false
	}
	return strings.Contains(link, `rel="next"`)
}

// parseError reads the error body from a non-2xx response and returns an APIError.
func (c *GitHubClient) parseError(resp *http.Response, url string) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(body))

	switch resp.StatusCode {
	case 401:
		msg = "authentication failed — check your GITHUB_TOKEN"
	case 403:
		if strings.Contains(msg, "rate limit") || resp.Header.Get("X-RateLimit-Remaining") == "0" {
			reset := resp.Header.Get("X-RateLimit-Reset")
			msg = "rate limit exceeded"
			if reset != "" {
				if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
					msg = fmt.Sprintf("rate limit exceeded — resets at %s",
						time.Unix(ts, 0).Format(time.RFC3339))
				}
			}
		} else {
			msg = "access forbidden — the repository may be private or you lack permissions"
		}
	case 404:
		msg = "PR not found — check the URL (the repo may be private or the PR doesn't exist)"
	case 410:
		msg = "gone — the PR has been deleted"
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    msg,
		URL:        url,
	}
}

// linkRE extracts the URL from a Link header value like <url>; rel="next".
var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// parseNextLink extracts the next page URL from the Link header.
// Currently unused but available for future use.
func parseNextLink(link string) string {
	m := linkRE.FindStringSubmatch(link)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}
