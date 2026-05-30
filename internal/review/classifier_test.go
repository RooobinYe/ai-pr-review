package review

import (
	"testing"
)

func TestClassifyFile_Code(t *testing.T) {
	tests := []string{
		"main.go",
		"src/app.py",
		"lib/utils.rs",
		"components/Button.tsx",
		"server.js",
		"pkg/handler.java",
		"src/main.kt",
	}
	for _, f := range tests {
		if cat := ClassifyFile(f); cat != FileCategoryCode {
			t.Errorf("ClassifyFile(%q) = %s, want code", f, cat.String())
		}
	}
}

func TestClassifyFile_Test(t *testing.T) {
	tests := []string{
		"main_test.go",
		"test_app.py",
		"test/app_test.py",
		"tests/test_utils.py",
		"__tests__/component.test.ts",
		"Component.spec.ts",
		"fixtures/data.json",
	}
	for _, f := range tests {
		if cat := ClassifyFile(f); cat != FileCategoryTest {
			t.Errorf("ClassifyFile(%q) = %s, want test", f, cat.String())
		}
	}
}

func TestClassifyFile_Config(t *testing.T) {
	tests := []string{
		"docker-compose.yml",
		"Dockerfile",
		"Makefile",
		".github/workflows/ci.yml",
		"config.toml",
		"deploy/k8s/service.yaml",
		"terraform/main.tf",
		"go.mod",
		".gitignore",
		"package-lock.json",
	}
	for _, f := range tests {
		if cat := ClassifyFile(f); cat != FileCategoryConfig {
			t.Errorf("ClassifyFile(%q) = %s, want config", f, cat.String())
		}
	}
}

func TestClassifyFile_Doc(t *testing.T) {
	tests := []string{
		"README.md",
		"docs/guide.md",
		"CHANGELOG.md",
		"LICENSE",
		"CONTRIBUTING.md",
	}
	for _, f := range tests {
		if cat := ClassifyFile(f); cat != FileCategoryDoc {
			t.Errorf("ClassifyFile(%q) = %s, want doc", f, cat.String())
		}
	}
}

func TestClassifyFile_Other(t *testing.T) {
	tests := []string{
		"assets/logo.png",
		"data.csv",
		"unknown.xyz",
	}
	for _, f := range tests {
		if cat := ClassifyFile(f); cat != FileCategoryOther {
			t.Errorf("ClassifyFile(%q) = %s, want other", f, cat.String())
		}
	}
}

func TestClassifyFiles(t *testing.T) {
	files := []string{"main.go", "main_test.go", "Dockerfile", "README.md"}
	result := ClassifyFiles(files)
	if len(result[FileCategoryCode]) != 1 {
		t.Errorf("expected 1 code file, got %d", len(result[FileCategoryCode]))
	}
	if len(result[FileCategoryTest]) != 1 {
		t.Errorf("expected 1 test file, got %d", len(result[FileCategoryTest]))
	}
	if len(result[FileCategoryConfig]) != 1 {
		t.Errorf("expected 1 config file, got %d", len(result[FileCategoryConfig]))
	}
	if len(result[FileCategoryDoc]) != 1 {
		t.Errorf("expected 1 doc file, got %d", len(result[FileCategoryDoc]))
	}
}

func TestSeverityString(t *testing.T) {
	if RiskSeverityCritical.String() != "critical" {
		t.Errorf("unexpected: %s", RiskSeverityCritical.String())
	}
	if RiskSeverityLow.String() != "low" {
		t.Errorf("unexpected: %s", RiskSeverityLow.String())
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected RiskSeverity
	}{
		{"critical", RiskSeverityCritical},
		{"high", RiskSeverityHigh},
		{"medium", RiskSeverityMedium},
		{"low", RiskSeverityLow},
		{"info", RiskSeverityInfo},
		{"unknown", RiskSeverityInfo},
	}
	for _, tt := range tests {
		if got := parseSeverity(tt.input); got != tt.expected {
			t.Errorf("parseSeverity(%q) = %s, want %s", tt.input, got.String(), tt.expected.String())
		}
	}
}
