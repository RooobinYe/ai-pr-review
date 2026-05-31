package review

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ai-pr-review/internal/pr"
	"ai-pr-review/internal/prompt"
)

// LLMClient abstracts the LLM API call for testability.
type LLMClient interface {
	SendMessage(ctx context.Context, systemPrompt, userMessage string) (string, error)
}

// llmRiskResponse is the JSON shape the LLM returns for risk analysis.
type llmRiskResponse struct {
	Summary     string       `json:"summary"`
	Limitations []string     `json:"limitations,omitempty"`
	FileChanges []fileChange `json:"file_changes"`
	Risks       []llmRisk    `json:"risks"`
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
	Confidence  string `json:"confidence"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Evidence    string `json:"evidence,omitempty"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
	Uncertainty string `json:"uncertainty,omitempty"`
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
		// If LLM call fails, return a result with classifier-only data but warn the user.
		fmt.Fprintf(os.Stderr, "Warning: AI analysis unavailable (%v).\n", err)
		fmt.Fprintln(os.Stderr, "         Showing classifier-only summary without risk analysis.")
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
		fmt.Fprintf(os.Stderr, "Warning: failed to parse AI response (%v).\n", err)
		fmt.Fprintln(os.Stderr, "         Showing classifier-only summary without risk analysis.")
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

	// Convert risks and sort by severity (most critical first).
	risks := convertRisks(llmResult.Risks)
	sortRisksBySeverity(risks)

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
// The prompt is now maintained in the prompt package for consistency.
func reviewSystemPrompt() string {
	return prompt.EngineSystemPrompt()
}

// buildReviewPrompt constructs the user message with PR context and diffs.
// Delegates to the centralised prompt builder.
func buildReviewPrompt(data *pr.PRData, diffs []pr.DiffFile) string {
	return prompt.EngineUserPrompt(data, diffs)
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
			Confidence:  parseConfidence(r.Confidence),
			Category:    r.Category,
			Title:       r.Title,
			Evidence:    r.Evidence,
			Description: r.Description,
			Suggestion:  r.Suggestion,
			Uncertainty: r.Uncertainty,
		})
	}
	return risks
}

// sortRisksBySeverity sorts risks in-place from most to least severe.
// Within the same severity, higher confidence items come first.
func sortRisksBySeverity(risks []Risk) {
	for i := 0; i < len(risks); i++ {
		for j := i + 1; j < len(risks); j++ {
			if risks[j].Severity < risks[i].Severity ||
				(risks[j].Severity == risks[i].Severity && risks[j].Confidence < risks[i].Confidence) {
				risks[i], risks[j] = risks[j], risks[i]
			}
		}
	}
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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
