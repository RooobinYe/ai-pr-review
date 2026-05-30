package review

import (
	"path/filepath"
	"strings"
)

// test file patterns
var testPatterns = []string{
	"_test.go", "_test.py", ".test.ts", ".spec.ts", ".test.js", ".spec.js",
	"_test.rs", "test_", ".test.java",
}

var testDirs = []string{"test/", "tests/", "__tests__/", "testdata/", "fixtures/"}

// config file patterns
var configNames = map[string]bool{
	"dockerfile": true, "makefile": true, "docker-compose.yml": true,
	"docker-compose.yaml": true, ".gitignore": true, ".dockerignore": true,
	".env": true, ".env.example": true, "go.mod": true, "go.sum": true,
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"cargo.lock": true, "gemfile.lock": true, "poetry.lock": true,
}

var configExts = map[string]bool{
	".yaml": true, ".yml": true, ".toml": true, ".cfg": true, ".ini": true,
	".tf": true, ".tfvars": true, ".hcl": true, ".json": true, ".xml": true,
	".gradle": true, ".properties": true, ".lock": true,
}

var configDirs = []string{".github/", ".circleci/", ".gitlab/", "ci/", "deploy/",
	"k8s/", "helm/", "terraform/", "ansible/", "config/", "configs/", "conf/"}

// doc file patterns
var docExts = map[string]bool{
	".md": true, ".markdown": true, ".rst": true, ".txt": true, ".adoc": true,
}

var docNames = map[string]bool{
	"license": true, "changelog": true, "contributing": true,
	"code_of_conduct": true, "security": true, "readme": true,
}

// code file extensions
var codeExts = map[string]bool{
	".go": true, ".rs": true, ".c": true, ".h": true, ".cpp": true, ".cc": true,
	".hpp": true, ".py": true, ".java": true, ".kt": true, ".swift": true,
	".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true,
	".rb": true, ".php": true, ".cs": true, ".scala": true, ".clj": true,
	".elm": true, ".hs": true, ".lua": true, ".sh": true, ".bash": true,
	".zsh": true, ".fish": true, ".ps1": true, ".sql": true, ".graphql": true,
	".proto": true, ".vue": true, ".svelte": true, ".html": true, ".css": true,
	".scss": true, ".less": true,
}

// ClassifyFile returns the category for a file based on its path and extension.
func ClassifyFile(filename string) FileCategory {
	base := strings.ToLower(filepath.Base(filename))
	dir := strings.ToLower(filepath.Dir(filename)) + "/"
	ext := strings.ToLower(filepath.Ext(filename))

	// Check test patterns first.
	for _, p := range testPatterns {
		if strings.Contains(base, p) || strings.HasSuffix(base, p) {
			return FileCategoryTest
		}
	}
	for _, d := range testDirs {
		if strings.HasPrefix(dir, d) || strings.Contains(dir, "/"+d) {
			return FileCategoryTest
		}
	}

	// Check doc patterns.
	if docExts[ext] {
		return FileCategoryDoc
	}
	if docNames[strings.TrimSuffix(base, ext)] {
		return FileCategoryDoc
	}

	// Check config patterns.
	if configNames[base] {
		return FileCategoryConfig
	}
	if configExts[ext] && !codeExts[ext] {
		return FileCategoryConfig
	}
	for _, d := range configDirs {
		if strings.HasPrefix(dir, d) || strings.Contains(dir, "/"+d) || strings.HasPrefix(base, ".") {
			// dotfiles are usually config
			if ext == "" || configExts[ext] {
				return FileCategoryConfig
			}
		}
	}

	// .github/, .circleci/ etc.
	if strings.HasPrefix(base, ".") && ext != ".go" && ext != ".rs" && ext != ".ts" && ext != ".js" {
		return FileCategoryConfig
	}

	// Check code patterns.
	if codeExts[ext] {
		return FileCategoryCode
	}

	return FileCategoryOther
}

// ClassifyFiles categorises a slice of filenames and returns a map.
func ClassifyFiles(files []string) map[FileCategory][]string {
	result := make(map[FileCategory][]string)
	for _, f := range files {
		cat := ClassifyFile(f)
		result[cat] = append(result[cat], f)
	}
	return result
}
