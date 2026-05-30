package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ai-pr-review/internal/pr"
)

// LLMClient abstracts the LLM API call for testability.
type LLMClient interface {
	SendMessage(ctx context.Context, systemPrompt, userMessage string) (string, error)
}

// llmRiskResponse is the JSON shape the LLM returns for risk analysis.
type llmRiskResponse struct {
	Summary      string        `json:"summary"`
	FileChanges  []fileChange  `json:"file_changes"`
	Risks        []llmRisk     `json:"risks"`
}

type fileChange struct {
	Filename string `json:"filename"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

type llmRisk struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// Engine runs the full review pipeline.
type Engine struct {
	client LLMClient
	model  string
}

// NewEngine creates a new review engine.
func NewEngine(client LLMClient, model string) *Engine {
	return &Engine{client: client, model: model}
}

// Run executes the full review pipeline on the given PR data.
func (e *Engine) Run(ctx context.Context, data *pr.PRData) (*ReviewResult, error) {
	diffs, err := pr.ParsePRData(data)
	if err != nil {
		return nil, fmt.Errorf("parse diffs: %w", err)
	}

	// Build file summaries from parsed data.
	fileSummaries := make([]FileSummary, 0, len(diffs))
	for _, df := range diffs {
		cat := ClassifyFile(df.Filename)
		adds, dels := countChanges(df)
		fs := FileSummary{
			Filename:  df.Filename,
			Category:  cat,
			Summary:   summariseDiff(&df),
			Additions: adds,
			Deletions: dels,
		}
		fileSummaries = append(fileSummaries, fs)
	}

	// Build prompt and call LLM for deeper analysis.
	prompt := buildReviewPrompt(data, diffs)
	systemPrompt := reviewSystemPrompt()

	response, err := e.client.SendMessage(ctx, systemPrompt, prompt)
	if err != nil {
		// If LLM call fails, return a result with classifier-only data.
		return &ReviewResult{
			PRInfo:      data.Info,
			Title:       data.Details.Title,
			Author:      data.Details.Author,
			BaseBranch:  data.Details.BaseBranch,
			HeadBranch:  data.Details.HeadBranch,
			FileChanges: fileSummaries,
			Summary:     fallbackSummary(data, fileSummaries),
		}, nil
	}

	// Parse LLM response.
	llmResult, err := parseLLMResponse(response)
	if err != nil {
		return &ReviewResult{
			PRInfo:      data.Info,
			Title:       data.Details.Title,
			Author:      data.Details.Author,
			BaseBranch:  data.Details.BaseBranch,
			HeadBranch:  data.Details.HeadBranch,
			FileChanges: fileSummaries,
			Summary:     fallbackSummary(data, fileSummaries),
		}, nil
	}

	// Merge LLM file summaries with classifier data.
	mergedFiles := mergeFileSummaries(fileSummaries, llmResult.FileChanges)

	// Convert risks.
	risks := convertRisks(llmResult.Risks)

	return &ReviewResult{
		PRInfo:      data.Info,
		Title:       data.Details.Title,
		Author:      data.Details.Author,
		BaseBranch:  data.Details.BaseBranch,
		HeadBranch:  data.Details.HeadBranch,
		FileChanges: mergedFiles,
		Risks:       risks,
		Summary:     llmResult.Summary,
	}, nil
}

// reviewSystemPrompt returns the system prompt for the review LLM call.
func reviewSystemPrompt() string {
	return `You are an expert code reviewer. Analyse the provided PR diff and return a JSON response with the following structure:

{
  "summary": "A concise 2-4 sentence overview of what this PR changes and why.",
  "file_changes": [
    {"filename": "...", "category": "code|test|config|doc|other", "summary": "one-line description"}
  ],
  "risks": [
    {
      "file": "path/to/file.go",
      "line": 0,
      "severity": "critical|high|medium|low|info",
      "category": "security|nil-pointer|error-handling|performance|logic|concurrency|style|other",
      "title": "short risk title",
      "description": "detailed explanation of the risk",
      "suggestion": "actionable fix suggestion"
    }
  ]
}

Guidelines:
- Only include real, concrete risks. Do not fabricate issues.
- Focus on: security vulnerabilities, nil pointer dereferences, missing error handling, race conditions, performance regressions, and logical errors.
- For each risk, provide a specific, actionable suggestion.
- Set line to the new file line number when possible (from the + lines in the diff), or 0 for file-level issues.
- If there are no significant risks, return an empty risks array.
- Return ONLY valid JSON, no markdown fences or commentary.`
}

// buildReviewPrompt constructs the user message with PR context and diffs.
func buildReviewPrompt(data *pr.PRData, diffs []pr.DiffFile) string {
	var b strings.Builder

	b.WriteString("## PR Information\n")
	b.WriteString(fmt.Sprintf("Title: %s\n", data.Details.Title))
	b.WriteString(fmt.Sprintf("Author: %s\n", data.Details.Author))
	b.WriteString(fmt.Sprintf("Branch: %s → %s\n", data.Details.HeadBranch, data.Details.BaseBranch))
	if data.Details.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n", truncate(data.Details.Description, 500)))
	}
	b.WriteString(fmt.Sprintf("\nFiles changed: %d\n\n", len(data.Files)))

	// File overview table.
	b.WriteString("## File Overview\n")
	for _, f := range data.Files {
		cat := ClassifyFile(f.Filename)
		tag := ""
		switch f.Status {
		case "added":
			tag = "[new]"
		case "modified":
			tag = "[mod]"
		case "removed":
			tag = "[del]"
		case "renamed":
			tag = "[ren]"
		}
		prev := ""
		if f.PreviousFilename != "" {
			prev = fmt.Sprintf(" (from %s)", f.PreviousFilename)
		}
		b.WriteString(fmt.Sprintf("- %s %s [%s] +%d/-%d%s\n",
			tag, f.Filename, cat.String(), f.Additions, f.Deletions, prev))
	}

	// Actual diffs (truncated if too large).
	b.WriteString("\n## Diffs\n")
	diffText := buildDiffText(diffs)
	if len(diffText) > 60000 {
		diffText = diffText[:60000] + "\n... (diff truncated, too large for full analysis)"
	}
	b.WriteString(diffText)

	return b.String()
}

// buildDiffText renders parsed diffs into a compact text format.
func buildDiffText(diffs []pr.DiffFile) string {
	var b strings.Builder
	for _, df := range diffs {
		b.WriteString(fmt.Sprintf("\n### %s", df.Filename))
		if df.PreviousFilename != "" {
			b.WriteString(fmt.Sprintf(" (renamed from %s)", df.PreviousFilename))
		}
		b.WriteString(fmt.Sprintf(" [%s]\n", df.Status))
		b.WriteString("```diff\n")
		for _, h := range df.Hunks {
			b.WriteString(h.Header + "\n")
			for _, l := range h.Lines {
				switch l.Type {
				case pr.DiffLineAdded:
					b.WriteString(fmt.Sprintf("+%s\n", l.Content))
				case pr.DiffLineRemoved:
					b.WriteString(fmt.Sprintf("-%s\n", l.Content))
				case pr.DiffLineContext:
					b.WriteString(fmt.Sprintf(" %s\n", l.Content))
				}
			}
		}
		b.WriteString("```\n")
		if b.Len() > 60000 {
			b.WriteString("\n... (remaining diffs truncated)\n")
			break
		}
	}
	return b.String()
}

// parseLLMResponse extracts JSON from the LLM response.
func parseLLMResponse(raw string) (*llmRiskResponse, error) {
	raw = strings.TrimSpace(raw)
	// Strip markdown fences if present.
	if strings.HasPrefix(raw, "```") {
		idx := strings.Index(raw, "\n")
		if idx >= 0 {
			raw = raw[idx+1:]
		}
		if strings.HasSuffix(raw, "```") {
			raw = raw[:len(raw)-3]
		}
		raw = strings.TrimSpace(raw)
	}

	var result llmRiskResponse
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse LLM JSON: %w", err)
	}
	return &result, nil
}

// convertRisks maps LLM risk strings to internal types.
func convertRisks(llmRisks []llmRisk) []Risk {
	risks := make([]Risk, 0, len(llmRisks))
	for _, r := range llmRisks {
		risks = append(risks, Risk{
			File:        r.File,
			Line:        r.Line,
			Severity:    parseSeverity(r.Severity),
			Category:    r.Category,
			Title:       r.Title,
			Description: r.Description,
			Suggestion:  r.Suggestion,
		})
	}
	return risks
}

// countChanges counts added and removed lines in a parsed DiffFile.
func countChanges(df pr.DiffFile) (adds, dels int) {
	for _, h := range df.Hunks {
		for _, l := range h.Lines {
			switch l.Type {
			case pr.DiffLineAdded:
				adds++
			case pr.DiffLineRemoved:
				dels++
			}
		}
	}
	return
}

// summariseDiff generates a short summary from parsed diff data.
func summariseDiff(df *pr.DiffFile) string {
	if df == nil || len(df.Hunks) == 0 {
		if df != nil && df.Status == "removed" {
			return "file removed"
		}
		return "no changes"
	}
	adds, dels := countChanges(*df)
	if df.Status == "added" {
		return fmt.Sprintf("new file: +%d lines", adds)
	}
	if df.Status == "removed" {
		return fmt.Sprintf("removed file: -%d lines", dels)
	}
	if df.Status == "renamed" {
		return fmt.Sprintf("renamed: +%d/-%d", adds, dels)
	}
	return fmt.Sprintf("+%d/-%d across %d hunks", adds, dels, len(df.Hunks))
}

// mergeFileSummaries merges classifier-based summaries with LLM-generated ones.
func mergeFileSummaries(base []FileSummary, llm []fileChange) []FileSummary {
	if len(llm) == 0 {
		return base
	}
	llmMap := make(map[string]string, len(llm))
	for _, f := range llm {
		llmMap[f.Filename] = f.Summary
	}
	for i := range base {
		if s, ok := llmMap[base[i].Filename]; ok && s != "" {
			base[i].Summary = s
		}
	}
	return base
}

// fallbackSummary generates a summary without LLM when the API call fails.
func fallbackSummary(data *pr.PRData, files []FileSummary) string {
	totalAdds, totalDels := 0, 0
	codeFiles, testFiles, configFiles, docFiles := 0, 0, 0, 0
	for _, f := range files {
		totalAdds += f.Additions
		totalDels += f.Deletions
		switch f.Category {
		case FileCategoryCode:
			codeFiles++
		case FileCategoryTest:
			testFiles++
		case FileCategoryConfig:
			configFiles++
		case FileCategoryDoc:
			docFiles++
		}
	}
	parts := []string{
		fmt.Sprintf("PR by %s changes %d file(s) with +%d/-%d lines.",
			data.Details.Author, len(files), totalAdds, totalDels),
	}
	if codeFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d code file(s) modified.", codeFiles))
	}
	if testFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d test file(s) modified.", testFiles))
	}
	if configFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d config file(s) modified.", configFiles))
	}
	if docFiles > 0 {
		parts = append(parts, fmt.Sprintf("%d doc file(s) modified.", docFiles))
	}
	return strings.Join(parts, " ")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
