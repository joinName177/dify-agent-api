package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var requiredFiles = []string{
	".github/pull_request_template.md",
	".github/workflows/ci.yaml",
	"AGENTS.md",
	"CONTRIBUTING.md",
	"README.md",
	"architecture-profile.json",
	"architecture-profile.schema.json",
	"docs/adr/0003-engineering-governance-toolchain.md",
	"docs/code-review.md",
	"docs/go-backend-architecture.md",
	"engineering-standards.md",
}

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
var governanceTargetPattern = regexp.MustCompile(`(?m)^governance-check\s*:`)

func main() {
	violations := checkRepository(".")
	if len(violations) == 0 {
		fmt.Println("Governance checks passed.")
		return
	}

	for _, violation := range violations {
		fmt.Fprintf(os.Stderr, "- %s\n", violation)
	}
	os.Exit(1)
}

func checkRepository(root string) []error {
	violations := make([]error, 0)

	for _, requiredFile := range requiredFiles {
		fileInfo, err := os.Stat(filepath.Join(root, requiredFile))
		if err != nil {
			violations = append(violations, fmt.Errorf("required file %q: %w", requiredFile, err))
		} else if !fileInfo.Mode().IsRegular() {
			violations = append(violations, fmt.Errorf("required path %q is not a regular file", requiredFile))
		}
	}

	makefilePath := filepath.Join(root, "Makefile")
	makefile, err := os.ReadFile(makefilePath)
	if err != nil {
		violations = append(violations, fmt.Errorf("read Makefile: %w", err))
	} else if !governanceTargetPattern.Match(makefile) {
		violations = append(violations, errors.New("makefile is missing governance-check target"))
	}

	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)

		if entry.IsDir() {
			if relativePath == ".git" {
				return filepath.SkipDir
			}
			if entry.Name() == ".cursor" {
				violations = append(violations, fmt.Errorf("tool-specific instruction directory is forbidden: %s", relativePath))
				return filepath.SkipDir
			}
			return nil
		}

		switch {
		case entry.Name() == "AGENTS.md" && relativePath != "AGENTS.md":
			violations = append(violations, fmt.Errorf("nested AGENTS.md is forbidden: %s", relativePath))
		case entry.Name() == "CLAUDE.md":
			violations = append(violations, fmt.Errorf("tool-specific instruction file is forbidden: %s", relativePath))
		case entry.Name() == "copilot-instructions.md":
			violations = append(violations, fmt.Errorf("tool-specific instruction file is forbidden: %s", relativePath))
		case strings.EqualFold(entry.Name(), "pull_request_template.md") && relativePath != ".github/pull_request_template.md":
			violations = append(violations, fmt.Errorf("duplicate pull request template is forbidden: %s", relativePath))
		case strings.HasPrefix(strings.ToLower(relativePath), ".github/pull_request_template/"):
			violations = append(violations, fmt.Errorf("duplicate pull request template is forbidden: %s", relativePath))
		}

		if filepath.Ext(relativePath) == ".md" {
			linkViolations, err := checkMarkdownLinks(root, relativePath)
			if err != nil {
				return err
			}
			violations = append(violations, linkViolations...)
		}

		return nil
	})
	if walkErr != nil {
		violations = append(violations, fmt.Errorf("walk repository: %w", walkErr))
	}

	sort.Slice(violations, func(left, right int) bool {
		return violations[left].Error() < violations[right].Error()
	})
	return violations
}

func checkMarkdownLinks(root, markdownPath string) ([]error, error) {
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(markdownPath)))
	if err != nil {
		return nil, fmt.Errorf("read markdown %s: %w", markdownPath, err)
	}

	violations := make([]error, 0)
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(string(contents), -1) {
		target := strings.Trim(strings.TrimSpace(match[1]), "<>")
		if isExternalOrAnchor(target) {
			continue
		}

		target = strings.SplitN(target, "#", 2)[0]
		decodedTarget, err := url.PathUnescape(target)
		if err != nil {
			violations = append(violations, fmt.Errorf("markdown %s has invalid link %q", markdownPath, target))
			continue
		}

		var resolvedPath string
		if strings.HasPrefix(decodedTarget, "/") {
			resolvedPath = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(decodedTarget, "/")))
		} else {
			resolvedPath = filepath.Join(root, filepath.Dir(filepath.FromSlash(markdownPath)), filepath.FromSlash(decodedTarget))
		}

		if _, err := os.Stat(resolvedPath); err != nil {
			violations = append(violations, fmt.Errorf("markdown %s links to missing path %q", markdownPath, target))
		}
	}
	return violations, nil
}

func isExternalOrAnchor(target string) bool {
	return target == "" || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "mailto:")
}
