package review

import (
	"encoding/json"
	"strings"

	"ai-pr-review/internal/pr"
)

// FileCategory classifies a changed file by its role in the project.
type FileCategory int

const (
	FileCategoryCode  FileCategory = iota // production source code
	FileCategoryTest                      // test files
	FileCategoryConfig                    // configuration, CI/CD, build scripts
	FileCategoryDoc                       // documentation, markdown
	FileCategoryOther                     // uncategorised
)

var categoryNames = map[FileCategory]string{
	FileCategoryCode:   "code",
	FileCategoryTest:   "test",
	FileCategoryConfig: "config",
	FileCategoryDoc:    "doc",
	FileCategoryOther:  "other",
}

func (c FileCategory) String() string {
	if s, ok := categoryNames[c]; ok {
		return s
	}
	return "other"
}

func (c FileCategory) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *FileCategory) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	for k, v := range categoryNames {
		if v == s {
			*c = k
			return nil
		}
	}
	*c = FileCategoryOther
	return nil
}

// RiskSeverity rates how serious a risk is.
type RiskSeverity int

const (
	RiskSeverityCritical RiskSeverity = iota
	RiskSeverityHigh
	RiskSeverityMedium
	RiskSeverityLow
	RiskSeverityInfo
)

var severityNames = map[RiskSeverity]string{
	RiskSeverityCritical: "critical",
	RiskSeverityHigh:     "high",
	RiskSeverityMedium:   "medium",
	RiskSeverityLow:      "low",
	RiskSeverityInfo:     "info",
}

func (s RiskSeverity) String() string {
	if n, ok := severityNames[s]; ok {
		return n
	}
	return "info"
}

func (s RiskSeverity) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *RiskSeverity) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = parseSeverity(str)
	return nil
}

// parseSeverity exported for JSON unmarshaling.
func parseSeverity(str string) RiskSeverity {
	lower := strings.ToLower(str)
	for k, v := range severityNames {
		if v == lower {
			return k
		}
	}
	return RiskSeverityInfo
}

// Risk represents a single risk found during review.
type Risk struct {
	File        string       `json:"file"`
	Line        int          `json:"line"` // 0 means file-level
	Severity    RiskSeverity `json:"severity"`
	Category    string       `json:"category"` // security, nil-pointer, error-handling, performance, etc.
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Suggestion  string       `json:"suggestion"`
}

// FileSummary summarises the changes in a single file.
type FileSummary struct {
	Filename  string       `json:"filename"`
	Category  FileCategory `json:"category"`
	Summary   string       `json:"summary"` // one-line description of changes
	Additions int          `json:"additions"`
	Deletions int          `json:"deletions"`
}

// ReviewResult is the complete output of the review pipeline.
type ReviewResult struct {
	PRInfo      *pr.PRInfo     `json:"pr_info"`
	Title       string         `json:"title"`
	Author      string         `json:"author"`
	BaseBranch  string         `json:"base_branch"`
	HeadBranch  string         `json:"head_branch"`
	FileChanges []FileSummary  `json:"file_changes"`
	Risks       []Risk         `json:"risks"`
	Summary     string         `json:"summary"` // overall PR summary paragraph
}
