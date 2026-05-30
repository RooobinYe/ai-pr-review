package review

import (
	"strings"
	"testing"

	"ai-pr-review/internal/pr"
)

func sampleResult() *ReviewResult {
	return &ReviewResult{
		PRInfo:     &pr.PRInfo{Owner: "owner", Repo: "repo", PullNumber: 1},
		Title:      "Test PR",
		Author:     "dev",
		BaseBranch: "main",
		HeadBranch: "feature",
		Summary:    "A test PR with changes.",
		FileChanges: []FileSummary{
			{Filename: "main.go", Category: FileCategoryCode, Summary: "added main func", Additions: 5, Deletions: 2},
			{Filename: "main_test.go", Category: FileCategoryTest, Summary: "added tests", Additions: 10, Deletions: 0},
		},
		Risks: []Risk{
			{
				File:        "main.go",
				Line:        4,
				Severity:    RiskSeverityMedium,
				Confidence:  ConfidenceMedium,
				Category:    "error-handling",
				Title:       "Unchecked error",
				Description: "Error from fmt.Println is not checked.",
				Suggestion:  "Check the returned error.",
			},
		},
	}
}

func TestFormatMarkdown(t *testing.T) {
	output := FormatMarkdown(sampleResult())

	// Check key sections exist.
	if !strings.Contains(output, "# PR Review Report") {
		t.Error("missing report title")
	}
	if !strings.Contains(output, "## PR Information") {
		t.Error("missing PR information section")
	}
	if !strings.Contains(output, "## Summary") {
		t.Error("missing summary section")
	}
	if !strings.Contains(output, "## Changed Files") {
		t.Error("missing changed files section")
	}
	if !strings.Contains(output, "## Risks") {
		t.Error("missing risks section")
	}
	if !strings.Contains(output, "Test PR") {
		t.Error("missing PR title")
	}
	if !strings.Contains(output, "@dev") {
		t.Error("missing author")
	}
	if !strings.Contains(output, "main.go") {
		t.Error("missing filename")
	}
	if !strings.Contains(output, "Unchecked error") {
		t.Error("missing risk title")
	}
	if !strings.Contains(output, "medium") {
		t.Error("missing severity")
	}
}

func TestFormatMarkdown_NoRisks(t *testing.T) {
	r := sampleResult()
	r.Risks = nil
	output := FormatMarkdown(r)
	if !strings.Contains(output, "No significant risks identified") {
		t.Error("missing no-risks message")
	}
}

func TestFormatJSON(t *testing.T) {
	output := FormatJSON(sampleResult())
	if !strings.Contains(output, `"title": "Test PR"`) {
		t.Error("missing title in JSON")
	}
	if !strings.Contains(output, `"severity": "medium"`) {
		t.Error("missing severity in JSON")
	}
	if !strings.Contains(output, `"file": "main.go"`) {
		t.Error("missing file in JSON")
	}
}

func TestFormatJSON_Empty(t *testing.T) {
	r := &ReviewResult{}
	output := FormatJSON(r)
	if !strings.Contains(output, `"title": ""`) {
		t.Error("expected empty title")
	}
}

func TestSeverityIcon(t *testing.T) {
	if !strings.Contains(severityIcon(RiskSeverityCritical), "critical") {
		t.Error("expected critical icon")
	}
	if !strings.Contains(severityIcon(RiskSeverityInfo), "info") {
		t.Error("expected info icon")
	}
}

func TestFormatMarkdown_ConfidenceSeparation(t *testing.T) {
	r := sampleResult()
	// Add a high-confidence critical risk and a low-confidence info risk.
	r.Risks = []Risk{
		{
			File: "critical.go", Line: 10, Severity: RiskSeverityCritical,
			Confidence: ConfidenceHigh, Category: "security",
			Title: "SQL injection", Description: "Raw SQL query.",
			Suggestion: "Use parameterized queries.",
		},
		{
			File: "main.go", Line: 4, Severity: RiskSeverityMedium,
			Confidence: ConfidenceMedium, Category: "error-handling",
			Title: "Unchecked error", Description: "Error not checked.",
			Suggestion: "Check the error.",
		},
		{
			File: "maybe.go", Line: 20, Severity: RiskSeverityLow,
			Confidence: ConfidenceLow, Category: "performance",
			Title: "Possible slow loop", Description: "Might be slow.",
			Suggestion: "Consider optimisation.",
		},
	}
	output := FormatMarkdown(r)

	// Should have both sections.
	if !strings.Contains(output, "Action Required") {
		t.Error("missing Action Required section")
	}
	if !strings.Contains(output, "Points of Interest") {
		t.Error("missing Points of Interest section")
	}

	// Action Required should contain high/medium confidence risks, not low.
	if !strings.Contains(output, "SQL injection") {
		t.Error("missing high-confidence risk in Action Required")
	}
	if !strings.Contains(output, "Unchecked error") {
		t.Error("missing medium-confidence risk in Action Required")
	}

	// Points of Interest should contain low confidence risk.
	if !strings.Contains(output, "Possible slow loop") {
		t.Error("missing low-confidence risk in Points of Interest")
	}

	// Confidence levels should be shown.
	if !strings.Contains(output, "Confidence") {
		t.Error("missing Confidence field in output")
	}
}

func TestFormatMarkdown_OnlyLowConfidence(t *testing.T) {
	r := sampleResult()
	r.Risks = []Risk{
		{
			File: "speculative.go", Line: 1, Severity: RiskSeverityInfo,
			Confidence: ConfidenceLow, Category: "style",
			Title: "Style nit", Description: "Minor style concern.",
			Suggestion: "Consider renaming.",
		},
	}
	output := FormatMarkdown(r)

	// Should NOT have Action Required when all risks are low confidence.
	if strings.Contains(output, "Action Required") {
		t.Error("should not have Action Required when all risks are low confidence")
	}
	// Should have Points of Interest.
	if !strings.Contains(output, "Points of Interest") {
		t.Error("should have Points of Interest for low confidence risks")
	}
}

func TestFormatMarkdown_OnlyHighConfidence(t *testing.T) {
	r := sampleResult()
	r.Risks = []Risk{
		{
			File: "bug.go", Line: 5, Severity: RiskSeverityHigh,
			Confidence: ConfidenceHigh, Category: "nil-pointer",
			Title: "Nil deref", Description: "Possible nil pointer.",
			Suggestion: "Add nil check.",
		},
	}
	output := FormatMarkdown(r)

	// Should have Action Required.
	if !strings.Contains(output, "Action Required") {
		t.Error("should have Action Required for high confidence risks")
	}
	// Should NOT have Points of Interest.
	if strings.Contains(output, "Points of Interest") {
		t.Error("should not have Points of Interest when no low confidence risks")
	}
}

func TestConfidenceIcon(t *testing.T) {
	if !strings.Contains(confidenceIcon(ConfidenceHigh), "high") {
		t.Error("expected high confidence icon")
	}
	if !strings.Contains(confidenceIcon(ConfidenceMedium), "medium") {
		t.Error("expected medium confidence icon")
	}
	if !strings.Contains(confidenceIcon(ConfidenceLow), "low") {
		t.Error("expected low confidence icon")
	}
}
