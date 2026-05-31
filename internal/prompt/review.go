// Package prompt centralises all LLM prompt templates for the AI PR review system.
//
// Design principles (informed by Claude Code prompt engineering research):
//  1. XML tags separate concerns — role, failure modes, anti-hallucination rules, criteria, output format
//  2. Static content (rules, criteria) precedes dynamic content (PR info, diffs) to enable prompt caching
//  3. Failure modes are declared explicitly so the model knows its own blind spots
//  4. Numerical anchors ("at most 15 findings", "at most 3 sentences") constrain output quality
//  5. Evidence requirements and uncertainty marking prevent hallucinated findings
package prompt

import (
	"fmt"
	"strings"

	"ai-pr-review/internal/pr"
)

// =============================================================================
// Agentic Review Mode (TUI / ConversationLoop with tools)
// =============================================================================

// AgenticReviewSystemPrompt returns the system prompt for the agentic review
// path (TUI mode). The model has access to tools (read_file, grep, glob, bash)
// and can explore the full cloned repository.
//
// Caching note: place static rules before dynamic PR context. The static portion
// (everything here) is a good candidate for Anthropic prompt caching.
func AgenticReviewSystemPrompt() string {
	return `You are an expert software security and code quality reviewer. Your task is to perform a rigorous, evidence-based code review of a GitHub pull request. You have access to the cloned repository and can explore it with read_file, grep, glob, and bash tools.

<review_criteria>
Evaluate the PR against these criteria, in priority order:

1. SECURITY: Code injection (SQL, command, XSS), authentication/authorization bypass, hardcoded secrets, unsafe deserialization, path traversal, sensitive data exposure.
2. CORRECTNESS: Logic errors, off-by-one, nil/null pointer dereferences, incorrect error propagation, type mismatches, broken control flow.
3. ERROR_HANDLING: Unhandled error returns, swallowed errors, overly broad catch clauses, missing error context, improper use of panic/abort.
4. CONCURRENCY: Race conditions, deadlocks, goroutine/thread leaks, missing synchronisation, shared mutable state without protection.
5. PERFORMANCE: N+1 queries, unnecessary allocations, blocking operations in hot paths, inefficient data structures.
6. MAINTAINABILITY: Unclear naming, missing comments on complex logic, excessive function length, duplicated code, breaking API changes.
7. TESTING: Missing tests for new functionality, tests that do not assert meaningful behaviour, brittle assertions.
</review_criteria>

<failure_modes>
The following are known failure modes of code review. Acknowledge these explicitly when they apply to your analysis:

1. INCOMPLETE_CONTEXT: Code outside the diff hunks (unmodified functions, imports, dependencies) is invisible by default. Always use read_file on changed files before making definitive claims.
2. DEPENDENCY_CHAIN_BLINDNESS: Changes in one file may have cascading effects in files not included in the diff. Use grep and glob to trace callers and dependents before claiming a change is safe.
3. UNKNOWN_CONVENTIONS: Project-specific conventions (naming, error-handling patterns, DI frameworks) may differ from standard patterns. Flag conventions that look unusual but acknowledge they may be intentional.
4. DYNAMIC_BEHAVIOUR_BLINDNESS: Static analysis cannot detect issues that only manifest at runtime (race conditions, deadlocks, panics in error paths, memory leaks). Use medium or low confidence for findings relying on runtime assumptions.
5. TRUNCATION_RISK: For very large PRs, you may not be able to read every file. Prioritise the most impactful changes and note any files you did not review.
</failure_modes>

<anti_hallucination_rules>
To prevent false findings, strictly follow these rules:

1. EVIDENCE_REQUIRED: Every risk or issue you report MUST include a specific, verifiable code snippet or diff excerpt from the actual code you read. If you cannot find concrete evidence, do NOT report the finding.
2. UNCERTAIN_MARKING: If you identify a potential issue but cannot verify it with available context, explicitly mark it as "[UNCERTAIN - requires human verification]" and set confidence to low.
3. NO_INVENTION: Do NOT invent function signatures, variable names, file paths, or code that you have not actually read. If you are uncertain about a name, use qualifying language like "possibly" or "likely."
4. BALANCE_CHECK: For every critical or high-severity finding, ask yourself: "Is there a reasonable alternative interpretation where this code is actually correct?" If yes, downgrade the severity or at minimum note the alternative.
5. QUANTITY_CAP: Report at most 15 findings total. If you find more potential issues, prioritise the most impactful ones. This forces you to distinguish signal from noise.
</anti_hallucination_rules>

<output_instructions>
Produce your final review as a structured Markdown document with these sections:

## PR Summary
- A 3-5 sentence overview of what this PR changes and why. Start with the most important change.

## Risk Analysis
For each risk, use this exact format:

### [Severity: critical|high|medium|low] [Confidence: high|medium|low] Short Title
- **File**: ` + "`path/to/file.go:line`" + `
- **Category**: security|correctness|error-handling|concurrency|performance|maintainability|testing
- **Evidence**: <specific code snippet or diff excerpt that demonstrates the issue>
- **Description**: <at most 3 sentences explaining the risk>
- **Suggestion**: <specific, actionable fix with code example where helpful>

## Suggestions
- Non-urgent improvements that are not risks but would improve code quality.

## Context Notes
- List any files you explored that are NOT in the diff but are relevant to your analysis.
- Note any limitations you encountered (truncated diff, inaccessible files, etc.).

CONSTRAINTS:
- Maximum 15 findings total across Risk Analysis and Suggestions.
- Each Description must be at most 3 sentences.
- Each Suggestion must include either a code example or a specific, actionable instruction.
- If no risks are found, explicitly state "No significant risks identified." and explain what you verified.
- After the final section, add a self-check line: "Self-check: I verified X of Y claims directly against the codebase; Z claims are based on diff analysis alone."
</output_instructions>`
}

// AgenticUserPrompt builds the user message for the agentic review path.
// extraArgs contains optional user instructions appended after the PR context
// (e.g. "使用中文回答", "focus on security issues").
func AgenticUserPrompt(data *pr.PRData, extraArgs string) string {
	var b strings.Builder

	b.WriteString("<context>\n")
	b.WriteString("The repository for this pull request has been cloned to your current working directory. ")
	b.WriteString("You can explore the full codebase using read_file, grep, glob, and bash tools.\n")
	b.WriteString("</context>\n\n")

	b.WriteString("<pr_info>\n")
	b.WriteString(fmt.Sprintf("**Title:** %s\n", data.Details.Title))
	b.WriteString(fmt.Sprintf("**Author:** %s\n", data.Details.Author))
	b.WriteString(fmt.Sprintf("**Branch:** `%s` → `%s`\n", data.Details.HeadBranch, data.Details.BaseBranch))
	if data.Details.Description != "" {
		b.WriteString(fmt.Sprintf("**Description:** %s\n", data.Details.Description))
	}
	b.WriteString(fmt.Sprintf("**Files changed:** %d (+%d/-%d)\n",
		len(data.Files),
		sumAdditions(data.Files),
		sumDeletions(data.Files)))
	b.WriteString("</pr_info>\n\n")

	b.WriteString("<changed_files>\n")
	for _, f := range data.Files {
		tag := statusTag(f.Status)
		cat := classifyFileName(f.Filename)
		b.WriteString(fmt.Sprintf("- %s `%s` [%s] +%d/-%d\n",
			tag, f.Filename, cat, f.Additions, f.Deletions))
	}
	b.WriteString("</changed_files>\n\n")

	b.WriteString("<exploration_checklist>\n")
	b.WriteString("Before writing your final review, you MUST complete this checklist. For each item, state what you found:\n\n")
	b.WriteString("1. READ at least 3 key changed files in full (use read_file). List them.\n")
	b.WriteString("2. TRACE at least 2 callers or dependents of changed functions (use grep). List what you found.\n")
	b.WriteString("3. CHECK if any tests exist for the changed code (use glob for *_test.* patterns). Report findings.\n")
	b.WriteString("4. REVIEW git log for recent related changes (use `git log --oneline -10`). Note any relevant context.\n")
	b.WriteString("5. IDENTIFY any breaking changes (API signature changes, config format changes, removed public symbols).\n\n")
	b.WriteString("If you cannot complete a checklist item, explain why (e.g., \"No callers found via grep\").\n")
	b.WriteString("</exploration_checklist>\n\n")

	b.WriteString("<review_request>\n")
	b.WriteString("Perform a thorough code review following the criteria specified in your system prompt. ")
	b.WriteString("Focus on the most impactful issues first. ")
	b.WriteString("Reference specific files and line numbers in your analysis.\n")
	b.WriteString("</review_request>\n")

	if extraArgs != "" {
		b.WriteString(fmt.Sprintf("\n<additional_instructions>\n%s\n</additional_instructions>\n", extraArgs))
	}

	return b.String()
}

// =============================================================================
// Engine Review Mode (--format markdown/json, single LLM call, no tools)
// =============================================================================

// EngineSystemPrompt returns the system prompt for the single-call review path.
// This path has NO access to tools — the model analyses only the provided diff.
func EngineSystemPrompt() string {
	return `You are an expert code reviewer performing a one-pass analysis of a PR diff. You do NOT have access to the full codebase — only the diff provided below.

<failure_modes>
The following limitations apply to this analysis. Acknowledge them when relevant:

1. DIFF_ONLY_ANALYSIS: You are analysing only the diff. You cannot see unmodified code, callers, implementations, or test suites. Findings that depend on broader codebase context may be inaccurate.
2. TRUNCATION_RISK: Diffs longer than 60,000 characters are truncated. If the diff cuts off mid-file, your analysis of that file is incomplete.
3. CONVENTION_BLINDNESS: Project-specific patterns (naming, error handling, dependency injection) may not be visible from the diff alone.
</failure_modes>

<anti_hallucination_rules>
1. EVIDENCE_REQUIRED: Only report issues you can point to in the provided diff. If you cannot cite a specific line, do not report it.
2. UNCERTAIN_MARKING: Mark speculative findings with confidence="low" and include an "uncertainty" field explaining the limitation.
3. NO_INVENTION: Do not invent variable names, function signatures, or code context not present in the diff.
4. QUANTITY_CAP: Report at most 15 risks. Prioritise the most impactful.
</anti_hallucination_rules>

<output_format>
Return ONLY a valid JSON object. No markdown fences, no commentary before or after. The JSON must follow this schema:

{
  "summary": "3-5 sentence overview of what this PR changes and why",
  "limitations": ["list of analysis limitations you encountered (use the failure mode names above)"],
  "file_changes": [
    {
      "filename": "path/to/file",
      "category": "code|test|config|doc|other",
      "summary": "one-line description of the change"
    }
  ],
  "risks": [
    {
      "file": "path/to/file.go",
      "line": 0,
      "severity": "critical|high|medium|low|info",
      "confidence": "high|medium|low",
      "category": "security|correctness|error-handling|concurrency|performance|maintainability|testing|style",
      "title": "short risk title (max 10 words)",
      "evidence": "specific diff line or snippet that demonstrates the issue",
      "description": "at most 3 sentences explaining the risk",
      "suggestion": "specific, actionable fix",
      "uncertainty": "required if confidence is medium or low; explain what additional context would increase confidence"
    }
  ]
}

RULES:
- Only include real, concrete risks with evidence from the diff.
- Set line to the new file line number (from + lines in the diff), or 0 for file-level issues.
- For confidence "low", the "uncertainty" field is REQUIRED.
- For confidence "medium", the "uncertainty" field is RECOMMENDED.
- If no significant risks exist, return an empty risks array and explain why in the summary.
- Each description must be at most 3 sentences.
- Each suggestion must be actionable (not vague like "improve this code").
- Maximum 15 risks.
</output_format>`
}

// EngineUserPrompt builds the user message for the single-call review path
// with the PR context and full diff text.
func EngineUserPrompt(data *pr.PRData, diffs []pr.DiffFile) string {
	var b strings.Builder

	// PR info section.
	b.WriteString("<pr_info>\n")
	b.WriteString(fmt.Sprintf("Title: %s\n", data.Details.Title))
	b.WriteString(fmt.Sprintf("Author: %s\n", data.Details.Author))
	b.WriteString(fmt.Sprintf("Branch: %s → %s\n", data.Details.HeadBranch, data.Details.BaseBranch))
	if data.Details.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n", truncate(data.Details.Description, 500)))
	}
	b.WriteString(fmt.Sprintf("Files changed: %d\n", len(data.Files)))
	b.WriteString("</pr_info>\n\n")

	// File overview.
	b.WriteString("<file_overview>\n")
	for _, f := range data.Files {
		tag := statusTag(f.Status)
		cat := classifyFileName(f.Filename)
		prev := ""
		if f.PreviousFilename != "" {
			prev = fmt.Sprintf(" (from %s)", f.PreviousFilename)
		}
		b.WriteString(fmt.Sprintf("- %s %s [%s] +%d/-%d%s\n",
			tag, f.Filename, cat, f.Additions, f.Deletions, prev))
	}
	b.WriteString("</file_overview>\n\n")

	// Diffs.
	b.WriteString("<diffs>\n")
	diffText := buildDiffText(diffs)
	if len(diffText) > 60000 {
		diffText = diffText[:60000] + "\n... (diff truncated at 60KB — analysis of later files may be incomplete)"
	}
	b.WriteString(diffText)
	b.WriteString("\n</diffs>\n\n")

	b.WriteString("<reminder>\n")
	b.WriteString("Remember: only report issues with verifiable evidence from the diffs above. ")
	b.WriteString("Mark speculative findings as confidence=\"low\" and include an uncertainty explanation. ")
	b.WriteString("Maximum 15 findings. Return valid JSON only — no markdown fences, no commentary.\n")
	b.WriteString("</reminder>\n")

	return b.String()
}

// =============================================================================
// Helpers shared across prompt builders
// =============================================================================

func statusTag(status string) string {
	switch status {
	case "added":
		return "[new]"
	case "modified":
		return "[mod]"
	case "removed":
		return "[del]"
	case "renamed":
		return "[ren]"
	default:
		return "[" + status + "]"
	}
}

// classifyFileName returns a category string for a filename.
// This mirrors the logic in internal/review/classifier.go so that prompt
// construction does not need to import the review package (avoiding a cycle).
func classifyFileName(filename string) string {
	// Test files.
	if strings.HasSuffix(filename, "_test.go") ||
		strings.HasSuffix(filename, "_test.py") ||
		strings.HasSuffix(filename, "_test.rs") ||
		strings.HasSuffix(filename, "_test.ts") ||
		strings.HasSuffix(filename, ".test.ts") ||
		strings.HasSuffix(filename, ".test.js") ||
		strings.HasSuffix(filename, ".spec.ts") ||
		strings.HasSuffix(filename, ".spec.js") ||
		strings.Contains(filename, "/test/") ||
		strings.Contains(filename, "/tests/") ||
		strings.Contains(filename, "/__tests__/") ||
		strings.Contains(filename, "/spec/") {
		return "test"
	}

	// Config files.
	configExts := []string{
		".json", ".yaml", ".yml", ".toml", ".ini", ".cfg", ".conf",
		".env", ".properties", ".hcl", ".tf", ".tfvars",
	}
	for _, ext := range configExts {
		if strings.HasSuffix(filename, ext) {
			return "config"
		}
	}
	configNames := []string{
		"Dockerfile", "Makefile", "CMakeLists.txt", "BUILD", "WORKSPACE",
		".gitignore", ".dockerignore", ".eslintrc", ".prettierrc",
		"go.mod", "go.sum", "package.json", "package-lock.json",
		"Cargo.toml", "Cargo.lock", "requirements.txt", "Pipfile",
	}
	base := filename
	if idx := strings.LastIndex(filename, "/"); idx >= 0 {
		base = filename[idx+1:]
	}
	for _, name := range configNames {
		if strings.EqualFold(base, name) {
			return "config"
		}
	}

	// Doc files.
	if strings.HasSuffix(filename, ".md") ||
		strings.HasSuffix(filename, ".rst") ||
		strings.HasSuffix(filename, ".txt") ||
		strings.HasSuffix(filename, ".adoc") {
		return "doc"
	}

	return "code"
}

func sumAdditions(files []pr.ChangedFile) int {
	total := 0
	for _, f := range files {
		total += f.Additions
	}
	return total
}

func sumDeletions(files []pr.ChangedFile) int {
	total := 0
	for _, f := range files {
		total += f.Deletions
	}
	return total
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// buildDiffText renders parsed diffs into a compact text format with diff syntax.
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
