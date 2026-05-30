package review

import (
	"context"
	"testing"

	"ai-pr-review/internal/pr"
)

// mockLLMClient implements LLMClient for testing.
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) SendMessage(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func samplePRData() *pr.PRData {
	return &pr.PRData{
		Info: &pr.PRInfo{Owner: "test", Repo: "test", PullNumber: 1},
		Details: &pr.PRDetails{
			Title:      "Add user auth",
			Author:     "dev",
			BaseBranch: "main",
			HeadBranch: "feature/auth",
			State:      "open",
		},
		Files: []pr.ChangedFile{
			{
				Filename:  "main.go",
				Status:    "modified",
				Additions: 5,
				Deletions: 2,
				Changes:   7,
				Patch:     "@@ -1,3 +1,6 @@\n package main\n+import \"fmt\"\n+func main() {\n+       fmt.Println(\"hello\")\n+}\n",
			},
		},
	}
}

func TestEngine_Run_Success(t *testing.T) {
	mock := &mockLLMClient{
		response: `{
  "summary": "This PR adds a main function with fmt import.",
  "file_changes": [
    {"filename": "main.go", "category": "code", "summary": "Added main function"}
  ],
  "risks": [
    {
      "file": "main.go",
      "line": 4,
      "severity": "low",
      "category": "error-handling",
      "title": "Missing error check",
      "description": "fmt.Println error is not checked.",
      "suggestion": "Consider checking the error return."
    }
  ]
}`,
	}

	engine := NewEngine(mock, "test-model")
	result, err := engine.Run(context.Background(), samplePRData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Add user auth" {
		t.Errorf("expected title 'Add user auth', got %q", result.Title)
	}
	if result.Author != "dev" {
		t.Errorf("expected author 'dev', got %q", result.Author)
	}
	if result.Summary != "This PR adds a main function with fmt import." {
		t.Errorf("unexpected summary: %q", result.Summary)
	}
	if len(result.FileChanges) != 1 {
		t.Fatalf("expected 1 file change, got %d", len(result.FileChanges))
	}
	if result.FileChanges[0].Filename != "main.go" {
		t.Errorf("expected main.go, got %s", result.FileChanges[0].Filename)
	}
	if len(result.Risks) != 1 {
		t.Fatalf("expected 1 risk, got %d", len(result.Risks))
	}
	if result.Risks[0].Title != "Missing error check" {
		t.Errorf("unexpected risk title: %q", result.Risks[0].Title)
	}
	if result.Risks[0].Severity != RiskSeverityLow {
		t.Errorf("expected low severity, got %s", result.Risks[0].Severity.String())
	}
	if result.Risks[0].Line != 4 {
		t.Errorf("expected line 4, got %d", result.Risks[0].Line)
	}
}

func TestEngine_Run_MalformedJSON_Survives(t *testing.T) {
	mock := &mockLLMClient{response: `not valid json`}
	engine := NewEngine(mock, "test")
	result, err := engine.Run(context.Background(), samplePRData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to classifier-only data.
	if len(result.FileChanges) != 1 {
		t.Errorf("expected 1 file change in fallback, got %d", len(result.FileChanges))
	}
	if result.Summary == "" {
		t.Error("expected non-empty fallback summary")
	}
}

func TestEngine_Run_LLMError_Survives(t *testing.T) {
	mock := &mockLLMClient{
		err: context.DeadlineExceeded,
	}
	engine := NewEngine(mock, "test")
	result, err := engine.Run(context.Background(), samplePRData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to classifier-only data.
	if result.FileChanges[0].Filename != "main.go" {
		t.Errorf("expected main.go, got %s", result.FileChanges[0].Filename)
	}
	if len(result.Risks) != 0 {
		t.Errorf("expected 0 risks on fallback, got %d", len(result.Risks))
	}
}

func TestParseLLMResponse_MarkdownFence(t *testing.T) {
	raw := "```json\n{\"summary\": \"test\", \"file_changes\": [], \"risks\": []}\n```"
	r, err := parseLLMResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Summary != "test" {
		t.Errorf("expected 'test', got %q", r.Summary)
	}
}

func TestParseLLMResponse_Plain(t *testing.T) {
	raw := `{"summary": "hello", "file_changes": [], "risks": []}`
	r, err := parseLLMResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Summary != "hello" {
		t.Errorf("expected 'hello', got %q", r.Summary)
	}
}

func TestCountChanges(t *testing.T) {
	df := pr.DiffFile{
		Hunks: []pr.DiffHunk{
			{
				Lines: []pr.DiffLine{
					{Type: pr.DiffLineContext},
					{Type: pr.DiffLineAdded},
					{Type: pr.DiffLineAdded},
					{Type: pr.DiffLineRemoved},
				},
			},
			{
				Lines: []pr.DiffLine{
					{Type: pr.DiffLineAdded},
					{Type: pr.DiffLineRemoved},
					{Type: pr.DiffLineRemoved},
				},
			},
		},
	}
	adds, dels := countChanges(df)
	if adds != 3 {
		t.Errorf("expected 3 additions, got %d", adds)
	}
	if dels != 3 {
		t.Errorf("expected 3 deletions, got %d", dels)
	}
}

func TestMergeFileSummaries(t *testing.T) {
	base := []FileSummary{
		{Filename: "a.go", Summary: "old summary", Category: FileCategoryCode},
		{Filename: "b.go", Summary: "old summary", Category: FileCategoryTest},
	}
	llm := []fileChange{
		{Filename: "a.go", Summary: "new summary"},
	}
	result := mergeFileSummaries(base, llm)
	if result[0].Summary != "new summary" {
		t.Errorf("expected 'new summary', got %q", result[0].Summary)
	}
	if result[1].Summary != "old summary" {
		t.Errorf("expected 'old summary', got %q", result[1].Summary)
	}
}

func TestFallbackSummary(t *testing.T) {
	data := samplePRData()
	files := []FileSummary{
		{Filename: "main.go", Category: FileCategoryCode, Additions: 5, Deletions: 2},
	}
	s := fallbackSummary(data, files)
	if s == "" {
		t.Error("expected non-empty fallback summary")
	}
}

func TestTruncate(t *testing.T) {
	s := truncate("hello world", 8)
	if s != "hello..." {
		t.Errorf("expected 'hello...', got %q", s)
	}
	s = truncate("short", 10)
	if s != "short" {
		t.Errorf("expected 'short', got %q", s)
	}
}
