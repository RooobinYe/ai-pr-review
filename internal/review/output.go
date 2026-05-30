package review

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatMarkdown renders a ReviewResult as a Markdown document.
func FormatMarkdown(r *ReviewResult) string {
	var b strings.Builder

	// Title
	b.WriteString("# PR Review Report\n\n")

	// PR header
	b.WriteString("## PR Information\n\n")
	b.WriteString(fmt.Sprintf("| Field | Value |\n|-------|-------|\n"))
	b.WriteString(fmt.Sprintf("| **Title** | %s |\n", r.Title))
	b.WriteString(fmt.Sprintf("| **Author** | @%s |\n", r.Author))
	b.WriteString(fmt.Sprintf("| **Branch** | `%s` → `%s` |\n", r.HeadBranch, r.BaseBranch))
	b.WriteString(fmt.Sprintf("| **Files** | %d changed |\n", len(r.FileChanges)))
	b.WriteString("\n")

	// Overall summary
	b.WriteString("## Summary\n\n")
	b.WriteString(r.Summary)
	b.WriteString("\n\n")

	// File changes
	b.WriteString("## Changed Files\n\n")
	b.WriteString("| Status | File | Category | Changes |\n|--------|------|----------|--------|\n")
	for _, f := range r.FileChanges {
		b.WriteString(fmt.Sprintf("| | `%s` | %s | %s |\n",
			f.Filename, f.Category.String(), f.Summary))
	}
	b.WriteString("\n")

	// Category breakdown
	b.WriteString("### By Category\n\n")
	catCount := make(map[FileCategory]int)
	catAdds := make(map[FileCategory]int)
	catDels := make(map[FileCategory]int)
	for _, f := range r.FileChanges {
		catCount[f.Category]++
		catAdds[f.Category] += f.Additions
		catDels[f.Category] += f.Deletions
	}
	b.WriteString("| Category | Files | +Additions | -Deletions |\n|----------|-------|------------|------------|\n")
	for _, cat := range []FileCategory{FileCategoryCode, FileCategoryTest, FileCategoryConfig, FileCategoryDoc, FileCategoryOther} {
		if n := catCount[cat]; n > 0 {
			b.WriteString(fmt.Sprintf("| %s | %d | +%d | -%d |\n",
				cat.String(), n, catAdds[cat], catDels[cat]))
		}
	}
	b.WriteString("\n")

	// Risks section
	b.WriteString("## Risks\n\n")
	if len(r.Risks) == 0 {
		b.WriteString("No significant risks identified.\n\n")
	} else {
		// Summary counts
		sevCount := make(map[RiskSeverity]int)
		for _, risk := range r.Risks {
			sevCount[risk.Severity]++
		}

		b.WriteString("| Severity | Count |\n|----------|-------|\n")
		for _, sev := range []RiskSeverity{RiskSeverityCritical, RiskSeverityHigh, RiskSeverityMedium, RiskSeverityLow, RiskSeverityInfo} {
			if n := sevCount[sev]; n > 0 {
				b.WriteString(fmt.Sprintf("| %s | %d |\n", severityIcon(sev), n))
			}
		}
		b.WriteString("\n")

		// Individual risks
		for i, risk := range r.Risks {
			b.WriteString(fmt.Sprintf("### %d. %s %s\n\n", i+1, severityIcon(risk.Severity), risk.Title))
			b.WriteString(fmt.Sprintf("- **Severity**: %s\n", risk.Severity.String()))
			b.WriteString(fmt.Sprintf("- **Category**: %s\n", risk.Category))
			b.WriteString(fmt.Sprintf("- **Location**: `%s`", risk.File))
			if risk.Line > 0 {
				b.WriteString(fmt.Sprintf(":%d", risk.Line))
			}
			b.WriteString("\n\n")
			b.WriteString(fmt.Sprintf("**Description**: %s\n\n", risk.Description))
			b.WriteString(fmt.Sprintf("**Suggestion**: %s\n\n", risk.Suggestion))
		}
	}

	return b.String()
}

// FormatJSON renders a ReviewResult as a formatted JSON string.
func FormatJSON(r *ReviewResult) string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal: %s"}`, err.Error())
	}
	return string(data)
}

// severityIcon returns an icon + label for a severity level.
func severityIcon(s RiskSeverity) string {
	switch s {
	case RiskSeverityCritical:
		return "🔴 critical"
	case RiskSeverityHigh:
		return "🟠 high"
	case RiskSeverityMedium:
		return "🟡 medium"
	case RiskSeverityLow:
		return "🟢 low"
	default:
		return "ℹ️ info"
	}
}
