package pr

import (
	"testing"
)

func TestParsePRURL_ValidHTTPS(t *testing.T) {
	info, err := ParsePRURL("https://github.com/owner/repo/pull/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Owner != "owner" {
		t.Errorf("expected owner=%q, got %q", "owner", info.Owner)
	}
	if info.Repo != "repo" {
		t.Errorf("expected repo=%q, got %q", "repo", info.Repo)
	}
	if info.PullNumber != 123 {
		t.Errorf("expected pullNumber=%d, got %d", 123, info.PullNumber)
	}
}

func TestParsePRURL_ValidHTTP(t *testing.T) {
	info, err := ParsePRURL("http://github.com/owner/repo/pull/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.PullNumber != 1 {
		t.Errorf("expected pullNumber=1, got %d", info.PullNumber)
	}
}

func TestParsePRURL_WithTrailingSlash(t *testing.T) {
	info, err := ParsePRURL("https://github.com/owner/repo/pull/123/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.PullNumber != 123 {
		t.Errorf("expected pullNumber=123, got %d", info.PullNumber)
	}
}

func TestParsePRURL_WithFilesSuffix(t *testing.T) {
	info, err := ParsePRURL("https://github.com/owner/repo/pull/123/files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.PullNumber != 123 {
		t.Errorf("expected pullNumber=123, got %d", info.PullNumber)
	}
}

func TestParsePRURL_WithWhitespace(t *testing.T) {
	info, err := ParsePRURL("  https://github.com/owner/repo/pull/456  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.PullNumber != 456 {
		t.Errorf("expected pullNumber=456, got %d", info.PullNumber)
	}
}

func TestParsePRURL_LargePRNumber(t *testing.T) {
	info, err := ParsePRURL("https://github.com/owner/repo/pull/999999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.PullNumber != 999999 {
		t.Errorf("expected pullNumber=999999, got %d", info.PullNumber)
	}
}

func TestParsePRURL_EmptyURL(t *testing.T) {
	_, err := ParsePRURL("")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestParsePRURL_InvalidScheme(t *testing.T) {
	_, err := ParsePRURL("ftp://github.com/owner/repo/pull/123")
	if err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}

func TestParsePRURL_NonGitHubHost(t *testing.T) {
	_, err := ParsePRURL("https://gitlab.com/owner/repo/pull/123")
	if err == nil {
		t.Fatal("expected error for non-github host")
	}
}

func TestParsePRURL_NotAPR(t *testing.T) {
	_, err := ParsePRURL("https://github.com/owner/repo/issues/123")
	if err == nil {
		t.Fatal("expected error for non-pull URL")
	}
}

func TestParsePRURL_NotANumber(t *testing.T) {
	_, err := ParsePRURL("https://github.com/owner/repo/pull/abc")
	if err == nil {
		t.Fatal("expected error for non-numeric pull number")
	}
}

func TestParsePRURL_ZeroPRNumber(t *testing.T) {
	_, err := ParsePRURL("https://github.com/owner/repo/pull/0")
	if err == nil {
		t.Fatal("expected error for zero pull number")
	}
}

func TestParsePRURL_NegativePRNumber(t *testing.T) {
	_, err := ParsePRURL("https://github.com/owner/repo/pull/-5")
	if err == nil {
		t.Fatal("expected error for negative pull number")
	}
}

func TestParsePRURL_TooShortPath(t *testing.T) {
	_, err := ParsePRURL("https://github.com/owner/repo/pull")
	if err == nil {
		t.Fatal("expected error for too-short path")
	}
}

func TestParsePRURL_MissingOwner(t *testing.T) {
	_, err := ParsePRURL("https://github.com//repo/pull/123")
	if err == nil {
		t.Fatal("expected error for missing owner")
	}
}
