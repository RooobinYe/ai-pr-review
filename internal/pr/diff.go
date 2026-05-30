package pr

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DiffLineType marks whether a line is context, added, or removed.
type DiffLineType int

const (
	DiffLineContext DiffLineType = iota
	DiffLineAdded
	DiffLineRemoved
)

// DiffLine is a single line within a hunk.
type DiffLine struct {
	Type    DiffLineType
	Content string
	OldNum  int // 1-based line number in the old file; 0 for added lines
	NewNum  int // 1-based line number in the new file; 0 for removed lines
}

// DiffHunk represents one contiguous change block in a file diff.
type DiffHunk struct {
	Header   string // raw @@ header line
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []DiffLine
}

// DiffFile represents the structured diff of a single changed file.
type DiffFile struct {
	Filename         string
	PreviousFilename string // set when the file was renamed
	Status           string // added, modified, removed, renamed
	Hunks            []DiffHunk
}

// hunkHeaderRE matches @@ -oldStart[,oldCount] +newStart[,newCount] @@ optional context.
var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)?$`)

// ParseDiff parses a unified-format diff string into structured hunks.
func ParseDiff(patch string) ([]DiffHunk, error) {
	scanner := bufio.NewScanner(strings.NewReader(patch))
	var hunks []DiffHunk
	var cur *DiffHunk
	oldLine, newLine := 0, 0

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "@@") {
			if cur != nil {
				hunks = append(hunks, *cur)
			}

			m := hunkHeaderRE.FindStringSubmatch(line)
			if m == nil {
				continue
			}

			oldStart, _ := strconv.Atoi(m[1])
			oldCount := 1
			if m[2] != "" {
				oldCount, _ = strconv.Atoi(m[2])
			}
			newStart, _ := strconv.Atoi(m[3])
			newCount := 1
			if m[4] != "" {
				newCount, _ = strconv.Atoi(m[4])
			}

			cur = &DiffHunk{
				Header:   line,
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
			}
			oldLine = oldStart
			newLine = newStart
			continue
		}

		if cur == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "+"):
			cur.Lines = append(cur.Lines, DiffLine{Type: DiffLineAdded, Content: line[1:], NewNum: newLine})
			newLine++
		case strings.HasPrefix(line, "-"):
			cur.Lines = append(cur.Lines, DiffLine{Type: DiffLineRemoved, Content: line[1:], OldNum: oldLine})
			oldLine++
		case strings.HasPrefix(line, " "):
			cur.Lines = append(cur.Lines, DiffLine{Type: DiffLineContext, Content: line[1:], OldNum: oldLine, NewNum: newLine})
			oldLine++
			newLine++
		case line == `\ No newline at end of file`:
			// informational marker — does not affect line counts
		default:
			// skip empty or unhandled lines
		}
	}

	if cur != nil {
		hunks = append(hunks, *cur)
	}

	return hunks, scanner.Err()
}

// ParseChangedFile parses the patch inside a ChangedFile into a DiffFile.
func ParseChangedFile(cf ChangedFile) (*DiffFile, error) {
	hunks, err := ParseDiff(cf.Patch)
	if err != nil {
		return nil, err
	}
	return &DiffFile{
		Filename:         cf.Filename,
		PreviousFilename: cf.PreviousFilename,
		Status:           cf.Status,
		Hunks:            hunks,
	}, nil
}

// ParsePRData parses all changed files in PRData into structured DiffFiles.
func ParsePRData(data *PRData) ([]DiffFile, error) {
	var result []DiffFile
	for _, f := range data.Files {
		df, err := ParseChangedFile(f)
		if err != nil {
			return nil, fmt.Errorf("failed to parse diff for %s: %w", f.Filename, err)
		}
		result = append(result, *df)
	}
	return result, nil
}
