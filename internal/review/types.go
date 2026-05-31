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

// ParseSeverity converts a string to a RiskSeverity (case-insensitive).
// Unknown strings return RiskSeverityInfo.
func ParseSeverity(str string) RiskSeverity {
	return parseSeverity(str)
}

// parseSeverity converts a string to a RiskSeverity.
func parseSeverity(str string) RiskSeverity {
	lower := strings.ToLower(str)
	for k, v := range severityNames {
		if v == lower {
			return k
		}
	}
	return RiskSeverityInfo
}

// ConfidenceLevel rates how confident the reviewer is about a finding.
type ConfidenceLevel int

const (
	ConfidenceHigh   ConfidenceLevel = iota
	ConfidenceMedium
	ConfidenceLow
)

var confidenceNames = map[ConfidenceLevel]string{
	ConfidenceHigh:   "high",
	ConfidenceMedium: "medium",
	ConfidenceLow:    "low",
}

func (c ConfidenceLevel) String() string {
	if n, ok := confidenceNames[c]; ok {
		return n
	}
	return "low"
}

func (c ConfidenceLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

func (c *ConfidenceLevel) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*c = parseConfidence(str)
	return nil
}

// parseConfidence converts a string to a ConfidenceLevel.
func parseConfidence(str string) ConfidenceLevel {
	lower := strings.ToLower(str)
	for k, v := range confidenceNames {
		if v == lower {
			return k
		}
	}
	return ConfidenceLow
}

// Risk represents a single risk found during review.
type Risk struct {
	File        string          `json:"file"`
	Line        int             `json:"line"` // 0 means file-level
	Severity    RiskSeverity    `json:"severity"`
	Confidence  ConfidenceLevel `json:"confidence"`
	Category    string          `json:"category"`              // security, nil-pointer, error-handling, performance, etc.
	Title       string          `json:"title"`
	Evidence    string          `json:"evidence,omitempty"`    // specific code snippet or diff excerpt
	Description string          `json:"description"`
	Suggestion  string          `json:"suggestion"`
	Uncertainty string          `json:"uncertainty,omitempty"` // explains low/medium confidence when applicable
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
	PRInfo      *pr.PRInfo    `json:"pr_info"`
	Title       string        `json:"title"`
	Author      string        `json:"author"`
	BaseBranch  string        `json:"base_branch"`
	HeadBranch  string        `json:"head_branch"`
	FileChanges []FileSummary `json:"file_changes"`
	Risks       []Risk        `json:"risks"`
	Summary     string        `json:"summary"` // overall PR summary paragraph
}

// CurrentSchemaVersion is bumped on breaking changes to the JSON output schema.
// Consumers should check this field to detect format drift.
const CurrentSchemaVersion = 2

// ReviewOutput is the top-level output structure for --format json.
// It differs from ReviewResult by splitting risks into must_fix / points_of_interest
// and including CI-friendly metadata (schema_version, tool_version, model, generated_at).
type ReviewOutput struct {
	SchemaVersion    int           `json:"schema_version"`
	ToolVersion      string        `json:"tool_version"`
	Model            string        `json:"model"`
	GeneratedAt      string        `json:"generated_at"`
	PRInfo           *pr.PRInfo    `json:"pr_info"`
	Title            string        `json:"title"`
	Author           string        `json:"author"`
	BaseBranch       string        `json:"base_branch"`
	HeadBranch       string        `json:"head_branch"`
	FileChanges      []FileSummary `json:"file_changes"`
	Summary          string        `json:"summary"`
	MustFix          []Risk        `json:"must_fix"`
	PointsOfInterest []Risk        `json:"points_of_interest"`
	Warnings         []string      `json:"warnings,omitempty"`
	RawReview        string        `json:"raw_review,omitempty"` // set only in fallback
}

// ErrorOutput is the structured error envelope emitted on failure.
// Always emitted to stdout so that CI pipelines can parse errors as JSON.
type ErrorOutput struct {
	SchemaVersion int         `json:"schema_version"`
	ToolVersion   string      `json:"tool_version"`
	GeneratedAt   string      `json:"generated_at"`
	Error         ErrorDetail `json:"error"`
}

// ErrorDetail carries a machine-readable error code and a human-readable message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
