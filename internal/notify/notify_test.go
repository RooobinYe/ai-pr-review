package notify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test-output.md")
	content := "# Review Results\n\nNo issues found."

	err := ExportToFile(testFile, content)
	if err != nil {
		t.Fatalf("ExportToFile() error = %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: got %q, want %q", string(data), content)
	}
}

func TestExportToFile_NestedPath(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "a", "b", "c", "output.json")
	content := `{"status": "ok"}`

	err := ExportToFile(testFile, content)
	if err != nil {
		t.Fatalf("ExportToFile() error = %v", err)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: got %q, want %q", string(data), content)
	}
}
