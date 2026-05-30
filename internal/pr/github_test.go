package pr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *GitHubClient) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client := &GitHubClient{
		Token: "test-token",
		HTTPClient: &http.Client{},
		BaseURL: srv.URL,
	}
	t.Cleanup(srv.Close)
	return srv, client
}

func TestGetPRDetails_Success(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/repos/owner/repo/pulls/123") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer token, got %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github.v3+json" {
			t.Errorf("expected v3 accept header, got %q", got)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"title":    "Add new feature",
			"body":     "This PR adds a new feature.\n\nFixes #42.",
			"state":    "open",
			"html_url": "https://github.com/owner/repo/pull/123",
			"user":     map[string]interface{}{"login": "devuser"},
			"base":     map[string]interface{}{"ref": "main", "sha": "abc123base"},
			"head":     map[string]interface{}{"ref": "feature-branch", "sha": "def456head"},
		})
	})

	details, err := client.GetPRDetails(context.Background(), "owner", "repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if details.Title != "Add new feature" {
		t.Errorf("expected title %q, got %q", "Add new feature", details.Title)
	}
	if details.Description != "This PR adds a new feature.\n\nFixes #42." {
		t.Errorf("unexpected description: %q", details.Description)
	}
	if details.Author != "devuser" {
		t.Errorf("expected author %q, got %q", "devuser", details.Author)
	}
	if details.State != "open" {
		t.Errorf("expected state %q, got %q", "open", details.State)
	}
	if details.BaseBranch != "main" {
		t.Errorf("expected base branch %q, got %q", "main", details.BaseBranch)
	}
	if details.HeadBranch != "feature-branch" {
		t.Errorf("expected head branch %q, got %q", "feature-branch", details.HeadBranch)
	}
	if details.BaseSHA != "abc123base" {
		t.Errorf("expected base SHA %q, got %q", "abc123base", details.BaseSHA)
	}
	if details.HeadSHA != "def456head" {
		t.Errorf("expected head SHA %q, got %q", "def456head", details.HeadSHA)
	}
}

func TestGetPRDetails_NotFound(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	})

	_, err := client.GetPRDetails(context.Background(), "owner", "repo", 999)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestGetPRDetails_Unauthorized(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	})

	_, err := client.GetPRDetails(context.Background(), "owner", "repo", 123)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !strings.Contains(apiErr.Message, "authentication failed") {
		t.Errorf("unexpected message: %s", apiErr.Message)
	}
}

func TestGetPRDetails_Forbidden(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "Resource not accessible by integration"}`))
	})

	_, err := client.GetPRDetails(context.Background(), "owner", "repo", 123)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !strings.Contains(apiErr.Message, "access forbidden") {
		t.Errorf("unexpected message: %s", apiErr.Message)
	}
}

func TestGetPRDetails_RateLimited(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "9999999999")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message": "API rate limit exceeded"}`))
	})

	_, err := client.GetPRDetails(context.Background(), "owner", "repo", 123)
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !strings.Contains(apiErr.Message, "rate limit exceeded") {
		t.Errorf("unexpected message: %s", apiErr.Message)
	}
}

func TestGetChangedFiles_Success(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"filename":   "main.go",
				"status":     "modified",
				"additions":  10,
				"deletions":  3,
				"changes":    13,
				"patch":      "@@ -1,5 +1,12 @@\n package main\n+import \"fmt\"\n func main() { fmt.Println(\"hello\") }",
			},
			{
				"filename":   "test.go",
				"status":     "added",
				"additions":  25,
				"deletions":  0,
				"changes":    25,
				"patch":      "@@ -0,0 +1,25 @@\n package main_test\n import \"testing\"",
			},
			{
				"filename":   "old.go",
				"status":     "removed",
				"additions":  0,
				"deletions":  15,
				"changes":    15,
				"patch":      "@@ -1,15 +0,0 @@\n package main\n // removed",
			},
			{
				"filename":          "renamed.go",
				"previous_filename": "original.go",
				"status":            "renamed",
				"additions":         2,
				"deletions":         2,
				"changes":           4,
				"patch":             "@@ -1,2 +1,2 @@\n-old\n+new",
			},
		})
	})

	files, err := client.GetChangedFiles(context.Background(), "owner", "repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(files))
	}

	if files[0].Filename != "main.go" || files[0].Status != "modified" || files[0].Additions != 10 || files[0].Deletions != 3 {
		t.Errorf("unexpected file[0]: %+v", files[0])
	}
	if files[1].Filename != "test.go" || files[1].Status != "added" || files[1].Additions != 25 {
		t.Errorf("unexpected file[1]: %+v", files[1])
	}
	if files[2].Status != "removed" {
		t.Errorf("expected status removed, got %s", files[2].Status)
	}
	if files[3].Status != "renamed" || files[3].PreviousFilename != "original.go" {
		t.Errorf("expected renamed from original.go, got %+v", files[3])
	}
}

func TestGetChangedFiles_Pagination(t *testing.T) {
	requestCount := 0
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			// First page: 2 files, with Link header for next page.
			w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/pulls/123/files?page=2>; rel="next"`)
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"filename": "a.go", "status": "modified", "additions": 1, "deletions": 0, "changes": 1, "patch": "@@ -1,1 +1,1 @@"},
				{"filename": "b.go", "status": "modified", "additions": 2, "deletions": 0, "changes": 2, "patch": "@@ -1,2 +1,2 @@"},
			})
		case 2:
			// Second page: 1 file, no Link header.
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"filename": "c.go", "status": "added", "additions": 3, "deletions": 0, "changes": 3, "patch": "@@ -0,0 +1,3 @@"},
			})
		default:
			t.Fatalf("unexpected request #%d", requestCount)
		}
	})

	files, err := client.GetChangedFiles(context.Background(), "owner", "repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files across pages, got %d", len(files))
	}
	if requestCount != 2 {
		t.Errorf("expected 2 requests for pagination, got %d", requestCount)
	}
}

func TestGetChangedFiles_Empty(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]map[string]interface{}{})
	})

	files, err := client.GetChangedFiles(context.Background(), "owner", "repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestGetChangedFiles_Error(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	})

	_, err := client.GetChangedFiles(context.Background(), "owner", "repo", 123)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestFetchPR_Success(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/files") {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"filename": "main.go", "status": "modified", "additions": 5, "deletions": 2, "changes": 7, "patch": "@@ -1,1 +1,1 @@"},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"title":    "Test PR",
				"body":     "Description",
				"state":    "open",
				"html_url": "https://github.com/owner/repo/pull/123",
				"user":     map[string]interface{}{"login": "author"},
				"base":     map[string]interface{}{"ref": "main", "sha": "abc"},
				"head":     map[string]interface{}{"ref": "feat", "sha": "def"},
			})
		}
	})

	data, err := client.FetchPR(context.Background(), "owner", "repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Info.Owner != "owner" || data.Info.Repo != "repo" || data.Info.PullNumber != 123 {
		t.Errorf("unexpected PR info: %+v", data.Info)
	}
	if data.Details.Title != "Test PR" {
		t.Errorf("expected title %q, got %q", "Test PR", data.Details.Title)
	}
	if len(data.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(data.Files))
	}
}

func TestFetchPR_DetailsError(t *testing.T) {
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := client.FetchPR(context.Background(), "owner", "repo", 123)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to fetch PR details") {
		t.Errorf("expected details error, got: %v", err)
	}
}

func TestFetchPR_FilesError(t *testing.T) {
	requestCount := 0
	_, client := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if strings.Contains(r.URL.Path, "/files") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"title":    "Test PR",
			"body":     "",
			"state":    "open",
			"html_url": "https://github.com/owner/repo/pull/123",
			"user":     map[string]interface{}{"login": "author"},
			"base":     map[string]interface{}{"ref": "main", "sha": "abc"},
			"head":     map[string]interface{}{"ref": "feat", "sha": "def"},
		})
	})

	_, err := client.FetchPR(context.Background(), "owner", "repo", 123)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to fetch changed files") {
		t.Errorf("expected files error, got: %v", err)
	}
}

func TestNewGitHubClient(t *testing.T) {
	client := NewGitHubClient("mytoken")
	if client.Token != "mytoken" {
		t.Errorf("expected token %q, got %q", "mytoken", client.Token)
	}
	if client.HTTPClient == nil {
		t.Error("expected non-nil HTTPClient")
	}
	if client.BaseURL != githubAPIBase {
		t.Errorf("expected base URL %q, got %q", githubAPIBase, client.BaseURL)
	}
	if client.HTTPClient.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
}

func TestNewGitHubClient_NoToken(t *testing.T) {
	client := NewGitHubClient("")
	if client.Token != "" {
		t.Errorf("expected empty token, got %q", client.Token)
	}
}

func TestParseNextLink(t *testing.T) {
	link := `<https://api.github.com/repos/owner/repo/pulls/123/files?page=2>; rel="next"`
	url := parseNextLink(link)
	if url != "https://api.github.com/repos/owner/repo/pulls/123/files?page=2" {
		t.Errorf("unexpected next link: %q", url)
	}
}

func TestParseNextLink_NoMatch(t *testing.T) {
	link := `<https://api.github.com/repos/owner/repo/pulls/123/files?page=1>; rel="prev"`
	url := parseNextLink(link)
	if url != "" {
		t.Errorf("expected empty, got %q", url)
	}
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 404, Message: "not found", URL: "https://api.github.com/test"}
	expected := "GitHub API error (404): not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
