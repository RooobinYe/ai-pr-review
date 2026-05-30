package pr

import (
	"strings"
	"testing"
)

func TestParseDiff_SingleHunk(t *testing.T) {
	patch := `@@ -1,4 +1,5 @@
 package main
+import "fmt"
 func main() {
-       oldCode()
+       newCode()
 }`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	h := hunks[0]
	if h.OldStart != 1 || h.OldCount != 4 {
		t.Errorf("unexpected old range: %d,%d", h.OldStart, h.OldCount)
	}
	if h.NewStart != 1 || h.NewCount != 5 {
		t.Errorf("unexpected new range: %d,%d", h.NewStart, h.NewCount)
	}
	if len(h.Lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(h.Lines))
	}

	if h.Lines[0].Type != DiffLineContext || h.Lines[0].OldNum != 1 || h.Lines[0].NewNum != 1 {
		t.Errorf("line 0: expected context (1/1), got type=%d old=%d new=%d", h.Lines[0].Type, h.Lines[0].OldNum, h.Lines[0].NewNum)
	}
	if h.Lines[1].Type != DiffLineAdded || h.Lines[1].NewNum != 2 || h.Lines[1].OldNum != 0 {
		t.Errorf("line 1: expected added (new=2), got type=%d old=%d new=%d", h.Lines[1].Type, h.Lines[1].OldNum, h.Lines[1].NewNum)
	}
	if h.Lines[2].Type != DiffLineContext || h.Lines[2].OldNum != 2 || h.Lines[2].NewNum != 3 {
		t.Errorf("line 2: expected context (2/3), got type=%d old=%d new=%d", h.Lines[2].Type, h.Lines[2].OldNum, h.Lines[2].NewNum)
	}
	if h.Lines[3].Type != DiffLineRemoved || h.Lines[3].OldNum != 3 || h.Lines[3].NewNum != 0 {
		t.Errorf("line 3: expected removed (old=3), got type=%d old=%d new=%d", h.Lines[3].Type, h.Lines[3].OldNum, h.Lines[3].NewNum)
	}
	if h.Lines[4].Type != DiffLineAdded || h.Lines[4].NewNum != 4 || h.Lines[4].OldNum != 0 {
		t.Errorf("line 4: expected added (new=4), got type=%d old=%d new=%d", h.Lines[4].Type, h.Lines[4].OldNum, h.Lines[4].NewNum)
	}
	if h.Lines[5].Type != DiffLineContext || h.Lines[5].OldNum != 4 || h.Lines[5].NewNum != 5 {
		t.Errorf("line 5: expected context (4/5), got type=%d old=%d new=%d", h.Lines[5].Type, h.Lines[5].OldNum, h.Lines[5].NewNum)
	}
}

func TestParseDiff_MultipleHunks(t *testing.T) {
	patch := `@@ -1,3 +1,3 @@
 a
-b
+c
 d
@@ -10,2 +10,3 @@
 x
+y
 z`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}

	if hunks[0].OldStart != 1 || hunks[0].NewStart != 1 {
		t.Errorf("hunk 0: expected (1,1), got (%d,%d)", hunks[0].OldStart, hunks[0].NewStart)
	}
	// 4 lines: context, removed, added, context
	if len(hunks[0].Lines) != 4 {
		t.Errorf("hunk 0: expected 4 lines, got %d", len(hunks[0].Lines))
	}

	if hunks[1].OldStart != 10 || hunks[1].NewStart != 10 {
		t.Errorf("hunk 1: expected (10,10), got (%d,%d)", hunks[1].OldStart, hunks[1].NewStart)
	}
	if len(hunks[1].Lines) != 3 {
		t.Errorf("hunk 1: expected 3 lines, got %d", len(hunks[1].Lines))
	}
	// Last line: context at old=11, new=12 (after +y at new=11)
	if hunks[1].Lines[2].OldNum != 11 || hunks[1].Lines[2].NewNum != 12 {
		t.Errorf("hunk 1 line 2: expected (11,12), got (%d,%d)", hunks[1].Lines[2].OldNum, hunks[1].Lines[2].NewNum)
	}
}

func TestParseDiff_NewFile(t *testing.T) {
	patch := `@@ -0,0 +1,3 @@
+package main
+import "fmt"
+func main() {}`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	h := hunks[0]
	if h.OldStart != 0 || h.OldCount != 0 {
		t.Errorf("expected old (0,0), got (%d,%d)", h.OldStart, h.OldCount)
	}
	if h.NewStart != 1 || h.NewCount != 3 {
		t.Errorf("expected new (1,3), got (%d,%d)", h.NewStart, h.NewCount)
	}

	addedCount := 0
	for _, l := range h.Lines {
		if l.Type != DiffLineAdded {
			t.Errorf("expected all added lines, got type=%d", l.Type)
		}
		addedCount++
	}
	if addedCount != 3 {
		t.Errorf("expected 3 added lines, got %d", addedCount)
	}
	// Verify line numbers are sequential.
	if h.Lines[0].NewNum != 1 || h.Lines[1].NewNum != 2 || h.Lines[2].NewNum != 3 {
		t.Errorf("expected new line numbers 1,2,3 got %d,%d,%d",
			h.Lines[0].NewNum, h.Lines[1].NewNum, h.Lines[2].NewNum)
	}
}

func TestParseDiff_RemovedFile(t *testing.T) {
	patch := `@@ -1,4 +0,0 @@
-package main
-func main() {
-       fmt.Println("bye")
-}`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}

	h := hunks[0]
	if h.OldStart != 1 || h.NewStart != 0 {
		t.Errorf("expected old=1 new=0, got old=%d new=%d", h.OldStart, h.NewStart)
	}

	for _, l := range h.Lines {
		if l.Type != DiffLineRemoved {
			t.Errorf("expected all removed lines, got type=%d", l.Type)
		}
	}
	if len(h.Lines) != 4 {
		t.Errorf("expected 4 removed lines, got %d", len(h.Lines))
	}
}

func TestParseDiff_NoNewlineAtEOF(t *testing.T) {
	patch := `@@ -1,2 +1,2 @@
 a
-b
\ No newline at end of file
+c
\ No newline at end of file`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	// Context(a) + removed(b) + context(c) = 3 lines. \ No newline markers are skipped.
	if len(hunks[0].Lines) != 3 {
		t.Fatalf("expected 3 lines (markers skipped), got %d", len(hunks[0].Lines))
	}
}

func TestParseDiff_EmptyPatch(t *testing.T) {
	hunks, err := ParseDiff("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 0 {
		t.Errorf("expected 0 hunks, got %d", len(hunks))
	}
}

func TestParseDiff_OnlyContext(t *testing.T) {
	patch := `@@ -10,3 +10,3 @@
 unchanged line 1
 unchanged line 2
 unchanged line 3`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	for _, l := range hunks[0].Lines {
		if l.Type != DiffLineContext {
			t.Errorf("expected all context lines, got type=%d", l.Type)
		}
	}
}

func TestParseDiff_DefaultCount(t *testing.T) {
	patch := `@@ -5 +5 @@
 context`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hunks[0].OldCount != 1 || hunks[0].NewCount != 1 {
		t.Errorf("expected (1,1) defaults, got (%d,%d)", hunks[0].OldCount, hunks[0].NewCount)
	}
}

func TestParseDiff_LineNumbersAcrossHunks(t *testing.T) {
	patch := `@@ -1,2 +1,3 @@
 a
+b
 c
@@ -10,2 +11,2 @@
 d
-e
+f`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d", len(hunks))
	}

	// First hunk line numbers.
	if hunks[0].Lines[0].OldNum != 1 || hunks[0].Lines[0].NewNum != 1 {
		t.Errorf("hunk0 line0: expected (1,1), got (%d,%d)", hunks[0].Lines[0].OldNum, hunks[0].Lines[0].NewNum)
	}
	if hunks[0].Lines[1].Type != DiffLineAdded || hunks[0].Lines[1].NewNum != 2 {
		t.Errorf("hunk0 line1: expected added new=2, got new=%d", hunks[0].Lines[1].NewNum)
	}
	if hunks[0].Lines[2].OldNum != 2 || hunks[0].Lines[2].NewNum != 3 {
		t.Errorf("hunk0 line2: expected (2,3), got (%d,%d)", hunks[0].Lines[2].OldNum, hunks[0].Lines[2].NewNum)
	}

	// Second hunk line numbers (reset from header: old=10, new=11).
	if hunks[1].Lines[0].OldNum != 10 || hunks[1].Lines[0].NewNum != 11 {
		t.Errorf("hunk1 line0: expected (10,11), got (%d,%d)", hunks[1].Lines[0].OldNum, hunks[1].Lines[0].NewNum)
	}
	if hunks[1].Lines[1].Type != DiffLineRemoved || hunks[1].Lines[1].OldNum != 11 {
		t.Errorf("hunk1 line1: expected removed old=11, got old=%d", hunks[1].Lines[1].OldNum)
	}
	if hunks[1].Lines[2].Type != DiffLineAdded || hunks[1].Lines[2].NewNum != 12 {
		t.Errorf("hunk1 line2: expected added new=12, got new=%d", hunks[1].Lines[2].NewNum)
	}
}

func TestParseDiff_HunkWithSectionHeader(t *testing.T) {
	patch := `@@ -25,5 +25,8 @@ func main() {
        var x int
+       x = 42
        fmt.Println(x)
 }`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if !strings.Contains(hunks[0].Header, "func main()") {
		t.Errorf("expected section header preserved, got: %s", hunks[0].Header)
	}
	// 4 lines: context, added, context, context
	if len(hunks[0].Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(hunks[0].Lines))
	}
}

func TestParseChangedFile(t *testing.T) {
	cf := ChangedFile{
		Filename: "main.go",
		Status:   "modified",
		Patch: `@@ -1,2 +1,2 @@
-old
+new`,
	}

	df, err := ParseChangedFile(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df.Filename != "main.go" {
		t.Errorf("expected main.go, got %s", df.Filename)
	}
	if df.Status != "modified" {
		t.Errorf("expected modified, got %s", df.Status)
	}
	if len(df.Hunks) != 1 {
		t.Errorf("expected 1 hunk, got %d", len(df.Hunks))
	}
}

func TestParseChangedFile_Renamed(t *testing.T) {
	cf := ChangedFile{
		Filename:         "new.go",
		PreviousFilename: "old.go",
		Status:           "renamed",
		Patch: `@@ -1,2 +1,2 @@
-old
+new`,
	}

	df, err := ParseChangedFile(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if df.PreviousFilename != "old.go" {
		t.Errorf("expected old.go, got %s", df.PreviousFilename)
	}
}

func TestParseChangedFile_EmptyPatch(t *testing.T) {
	cf := ChangedFile{
		Filename: "empty.go",
		Status:   "modified",
		Patch:    "",
	}

	df, err := ParseChangedFile(cf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(df.Hunks) != 0 {
		t.Errorf("expected 0 hunks, got %d", len(df.Hunks))
	}
}

func TestParsePRData(t *testing.T) {
	data := &PRData{
		Info:    &PRInfo{Owner: "o", Repo: "r", PullNumber: 1},
		Details: &PRDetails{Title: "test"},
		Files: []ChangedFile{
			{
				Filename: "a.go",
				Status:   "modified",
				Patch:    "@@ -1,1 +1,1 @@\n-old\n+new",
			},
			{
				Filename: "b.go",
				Status:   "added",
				Patch:    "@@ -0,0 +1,1 @@\n+package main",
			},
		},
	}

	files, err := ParsePRData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Filename != "a.go" {
		t.Errorf("expected a.go, got %s", files[0].Filename)
	}
	if files[1].Filename != "b.go" || files[1].Status != "added" {
		t.Errorf("unexpected file[1]: %+v", files[1])
	}
}

func TestParsePRData_Empty(t *testing.T) {
	data := &PRData{
		Info:    &PRInfo{Owner: "o", Repo: "r", PullNumber: 1},
		Details: &PRDetails{Title: "empty pr"},
		Files:   []ChangedFile{},
	}

	files, err := ParsePRData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestDiffLine_FileLineReference(t *testing.T) {
	patch := `@@ -10,0 +15,3 @@
+added line at 15
+added line at 16
+added line at 17`

	hunks, err := ParseDiff(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h := hunks[0]
	if h.Lines[0].NewNum != 15 {
		t.Errorf("expected new line 15, got %d", h.Lines[0].NewNum)
	}
	if h.Lines[1].NewNum != 16 {
		t.Errorf("expected new line 16, got %d", h.Lines[1].NewNum)
	}
	if h.Lines[2].NewNum != 17 {
		t.Errorf("expected new line 17, got %d", h.Lines[2].NewNum)
	}
}
