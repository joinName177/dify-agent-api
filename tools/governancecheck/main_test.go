package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRepositoryAcceptsValidLayout(t *testing.T) {
	root := createValidRepository(t)

	if violations := checkRepository(root); len(violations) != 0 {
		t.Fatalf("expected valid repository, got %v", violations)
	}
}

func TestCheckRepositoryRejectsMissingRequiredFile(t *testing.T) {
	root := createValidRepository(t)
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}

	assertViolationContains(t, checkRepository(root), `required file "AGENTS.md"`)
}

func TestCheckRepositoryRejectsDuplicateInstructions(t *testing.T) {
	root := createValidRepository(t)
	writeFile(t, root, "internal/AGENTS.md", "# duplicate\n")
	writeFile(t, root, "CLAUDE.md", "# duplicate\n")
	writeFile(t, root, "internal/module/.cursor/rules.md", "# duplicate\n")
	writeFile(t, root, ".github/copilot-instructions.md", "# duplicate\n")

	violations := checkRepository(root)
	assertViolationContains(t, violations, "nested AGENTS.md is forbidden")
	assertViolationContains(t, violations, "tool-specific instruction file is forbidden")
	assertViolationContains(t, violations, "tool-specific instruction directory is forbidden")
}

func TestCheckRepositoryRejectsDuplicatePullRequestTemplate(t *testing.T) {
	root := createValidRepository(t)
	writeFile(t, root, "pull_request_template.md", "# duplicate\n")
	writeFile(t, root, ".github/PULL_REQUEST_TEMPLATE/feature.md", "# duplicate\n")

	violations := checkRepository(root)
	assertViolationCount(t, violations, "duplicate pull request template is forbidden", 2)
}

func TestCheckRepositoryRejectsBrokenMarkdownLink(t *testing.T) {
	root := createValidRepository(t)
	writeFile(t, root, "README.md", "[missing](docs/missing.md)\n")

	assertViolationContains(t, checkRepository(root), "links to missing path")
}

func TestCheckRepositoryRequiresGovernanceTarget(t *testing.T) {
	root := createValidRepository(t)
	writeFile(t, root, "Makefile", "# governance-check:\ntest:\n\tgo test ./...\n")

	assertViolationContains(t, checkRepository(root), "makefile is missing governance-check target")
}

func TestCheckRepositoryRejectsRequiredDirectory(t *testing.T) {
	root := createValidRepository(t)
	path := filepath.Join(root, "AGENTS.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}

	assertViolationContains(t, checkRepository(root), "is not a regular file")
}

func createValidRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, requiredFile := range requiredFiles {
		writeFile(t, root, requiredFile, "# valid\n")
	}
	writeFile(t, root, "Makefile", "governance-check:\n\tgo test ./tools/governancecheck\n")
	return root
}

func writeFile(t *testing.T, root, relativePath, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertViolationContains(t *testing.T, violations []error, expected string) {
	t.Helper()
	for _, violation := range violations {
		if strings.Contains(violation.Error(), expected) {
			return
		}
	}
	t.Fatalf("expected violation containing %q, got %v", expected, violations)
}

func assertViolationCount(t *testing.T, violations []error, expected string, wanted int) {
	t.Helper()
	count := 0
	for _, violation := range violations {
		if strings.Contains(violation.Error(), expected) {
			count++
		}
	}
	if count != wanted {
		t.Fatalf("expected %d violations containing %q, got %d in %v", wanted, expected, count, violations)
	}
}
