package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	assignmentPattern = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*[:?]?=\s*(.+)$`)
	variablePattern   = regexp.MustCompile(`\$\(([A-Z][A-Z0-9_]*)\)`)
	digestPattern     = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	goImagePattern    = regexp.MustCompile(`^golang:([0-9]+\.[0-9]+\.[0-9]+)-`)
	actionPattern     = regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*([^\s#]+)(?:\s+#.*)?$`)
	runPattern        = regexp.MustCompile(`(?m)^\s+-\s+run:\s*(.+)$`)
)

func main() {
	root := "."
	if len(os.Args) == 2 {
		root = os.Args[1]
	} else if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: toolchaincheck [repository-root]")
		os.Exit(2)
	}
	violations := validate(root)
	if len(violations) > 0 {
		for _, violation := range violations {
			fmt.Fprintf(os.Stderr, "- %s\n", violation)
		}
		os.Exit(1)
	}
	fmt.Println("Toolchain consistency checks passed.")
}

func validate(root string) []string {
	files := map[string]string{}
	violations := []string{}
	for _, path := range []string{"Makefile", "go.mod", "Dockerfile", "README.md", ".github/workflows/ci.yaml", ".github/actions/go-cache/action.yml", "api/openapi/v1/oapi-codegen.yaml"} {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			violations = append(violations, fmt.Sprintf("read %s: %v", path, err))
			continue
		}
		files[path] = string(content)
	}
	if len(violations) > 0 {
		return violations
	}

	variables := parseMakeVariables(files["Makefile"])
	requiredVariables := []string{"GO_IMAGE", "RACE_GO_IMAGE", "OAPI_CODEGEN_VERSION", "MIGRATE_VERSION", "MIGRATE_IMAGE", "STATICCHECK_VERSION", "GOVULNCHECK_VERSION", "GITLEAKS_VERSION", "GITLEAKS_IMAGE", "GOSEC_VERSION", "GO_LICENSES_VERSION", "GRYPE_VERSION", "GRYPE_IMAGE"}
	for _, name := range requiredVariables {
		if variables[name] == "" {
			violations = append(violations, "Makefile variable is missing: "+name)
		}
	}

	toolchain := lineValue(files["go.mod"], "toolchain go")
	goDirective := lineValue(files["go.mod"], "go ")
	for _, name := range []string{"GO_IMAGE", "RACE_GO_IMAGE"} {
		matches := goImagePattern.FindStringSubmatch(variables[name])
		if len(matches) != 2 {
			violations = append(violations, fmt.Sprintf("%s must be a digest-pinned golang patch image", name))
			continue
		}
		if matches[1] != toolchain {
			violations = append(violations, fmt.Sprintf("%s uses Go %s but go.mod toolchain uses %s", name, matches[1], toolchain))
		}
		if !digestPattern.MatchString(variables[name]) {
			violations = append(violations, name+" is not pinned by sha256 digest")
		}
	}
	if majorMinor(toolchain) != majorMinor(goDirective) {
		violations = append(violations, fmt.Sprintf("go.mod go directive %s is incompatible with toolchain %s", goDirective, toolchain))
	}
	if !strings.Contains(files["Dockerfile"], "FROM "+variables["GO_IMAGE"]+" AS build") {
		violations = append(violations, "Dockerfile build image differs from GO_IMAGE")
	}
	if !strings.Contains(files[".github/workflows/ci.yaml"], "container: "+variables["RACE_GO_IMAGE"]) {
		violations = append(violations, "CI quality container differs from RACE_GO_IMAGE")
	}
	if !strings.Contains(files["README.md"], "`toolchain go"+toolchain+"`") {
		violations = append(violations, "README Go toolchain version differs from go.mod")
	}

	for _, name := range []string{"MIGRATE_IMAGE", "GITLEAKS_IMAGE", "GRYPE_IMAGE"} {
		if !digestPattern.MatchString(variables[name]) {
			violations = append(violations, name+" is not pinned by sha256 digest")
		}
	}
	if !strings.Contains(variables["MIGRATE_IMAGE"], ":"+variables["MIGRATE_VERSION"]+"@sha256:") {
		violations = append(violations, "MIGRATE_IMAGE tag differs from MIGRATE_VERSION")
	}
	if !strings.Contains(variables["GITLEAKS_IMAGE"], ":"+variables["GITLEAKS_VERSION"]+"@sha256:") {
		violations = append(violations, "GITLEAKS_IMAGE tag differs from GITLEAKS_VERSION")
	}
	if !strings.Contains(variables["GRYPE_IMAGE"], ":"+variables["GRYPE_VERSION"]+"@sha256:") {
		violations = append(violations, "GRYPE_IMAGE tag differs from GRYPE_VERSION")
	}
	for _, required := range []string{
		"oapi-codegen@$(OAPI_CODEGEN_VERSION)",
		"staticcheck@$(STATICCHECK_VERSION)",
		"govulncheck@$(GOVULNCHECK_VERSION)",
		"migrate@$(MIGRATE_VERSION)",
		"GITLEAKS_IMAGE='$(GITLEAKS_IMAGE)'",
		"gosec@$(GOSEC_VERSION)",
		"go-licenses/v2@$(GO_LICENSES_VERSION)",
		"GRYPE_IMAGE='$(GRYPE_IMAGE)'",
	} {
		if !strings.Contains(files["Makefile"], required) {
			violations = append(violations, "Makefile tool invocation does not use its owner variable: "+required)
		}
	}
	for scanner := bufio.NewScanner(strings.NewReader(files["Dockerfile"])); scanner.Scan(); {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# syntax=") || strings.HasPrefix(line, "FROM ") {
			reference := strings.Fields(strings.TrimPrefix(strings.TrimPrefix(line, "# syntax="), "FROM "))[0]
			if !digestPattern.MatchString(reference) {
				violations = append(violations, "Dockerfile image is not digest pinned: "+reference)
			}
		}
	}

	for _, actionFile := range []string{".github/workflows/ci.yaml", ".github/actions/go-cache/action.yml"} {
		for _, match := range actionPattern.FindAllStringSubmatch(files[actionFile], -1) {
			reference := match[1]
			if strings.HasPrefix(reference, "./") {
				continue
			}
			parts := strings.Split(reference, "@")
			if len(parts) != 2 || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(parts[1]) {
				violations = append(violations, actionFile+" Action is not pinned to a commit SHA: "+reference)
			}
		}
	}
	cacheAction := files[".github/actions/go-cache/action.yml"]
	for _, required := range []string{"/go/pkg/mod", "/root/.cache/go-build", "${{ runner.os }}-${{ runner.arch }}", "hashFiles('go.sum', 'Makefile')"} {
		if !strings.Contains(cacheAction, required) {
			violations = append(violations, "Go cache action is missing safe cache contract: "+required)
		}
	}
	if count := strings.Count(files[".github/workflows/ci.yaml"], "uses: ./.github/actions/go-cache"); count != 10 {
		violations = append(violations, fmt.Sprintf("CI must use the shared Go cache in exactly 10 Go jobs, found %d", count))
	}
	if count := strings.Count(files[".github/workflows/ci.yaml"], "uses: actions/checkout@"); count != 11 {
		violations = append(violations, fmt.Sprintf("CI must checkout exactly once in each of 11 active jobs, found %d", count))
	}
	for scanner := bufio.NewScanner(strings.NewReader(files[".github/workflows/ci.yaml"])); scanner.Scan(); {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "container: ") || strings.HasPrefix(line, "image: ") {
			reference := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if !digestPattern.MatchString(reference) {
				violations = append(violations, "CI container image is not digest pinned: "+reference)
			}
			if strings.HasPrefix(line, "container: ") && reference != variables["RACE_GO_IMAGE"] {
				violations = append(violations, "CI Go job container differs from RACE_GO_IMAGE: "+reference)
			}
		}
	}

	if !strings.Contains(files["api/openapi/v1/oapi-codegen.yaml"], "/"+variables["OAPI_CODEGEN_VERSION"]+"/configuration-schema.json") {
		violations = append(violations, "oapi-codegen schema version differs from OAPI_CODEGEN_VERSION")
	}
	for _, forbidden := range []string{"oapi-codegen@", "staticcheck@", "govulncheck@", "migrate@"} {
		if strings.Contains(files[".github/workflows/ci.yaml"], forbidden) {
			violations = append(violations, "CI owns a duplicate tool version: "+forbidden)
		}
	}
	requiredChecks := []string{"governance-check", "format-check", "generated-code-drift", "module-check", "static-analysis", "architecture-check", "unit-race-test", "migration-check", "integration-test", "e2e-test", "container-build"}
	for _, checkID := range requiredChecks {
		jobDefinition := "\n  " + checkID + ":\n    name: " + checkID + "\n"
		if !strings.Contains(files[".github/workflows/ci.yaml"], jobDefinition) {
			violations = append(violations, "CI stable Check ID is missing or has a different display name: "+checkID)
		}
	}
	requiredInvocations := []string{
		"make ci-governance-check GO_RUNNER=",
		"make format-check GO_RUNNER=",
		"make generated-check OAPI_CODEGEN_RUNTIME=local",
		"make mod-check GO_RUNNER=",
		"make static-analysis GO_RUNNER=",
		"make architecture-check GO_RUNNER=",
		"make unit-race-test GO_RUNNER= RACE_RUNNER=",
		"make migration-check GO_RUNNER=",
		"make integration-test GO_RUNNER=",
		"make e2e-test GO_RUNNER=",
		"make container-build IMAGE_TAG=dify-agent-api:ci",
	}
	for _, required := range requiredInvocations {
		if !strings.Contains(files[".github/workflows/ci.yaml"], required) {
			violations = append(violations, "CI stable Make invocation is missing: "+required)
		}
	}
	runs := runPattern.FindAllStringSubmatch(files[".github/workflows/ci.yaml"], -1)
	if len(runs) != len(requiredInvocations) {
		violations = append(violations, fmt.Sprintf("CI must contain exactly %d single-line run steps, found %d", len(requiredInvocations), len(runs)))
	}
	for _, run := range runs {
		if !strings.HasPrefix(strings.TrimSpace(run[1]), "make ") {
			violations = append(violations, "CI run step bypasses a stable Make target: "+strings.TrimSpace(run[1]))
		}
	}

	sort.Strings(violations)
	return violations
}

func parseMakeVariables(content string) map[string]string {
	variables := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		matches := assignmentPattern.FindStringSubmatch(line)
		if len(matches) == 3 {
			variables[matches[1]] = strings.TrimSpace(matches[2])
		}
	}
	for iteration := 0; iteration < 10; iteration++ {
		changed := false
		for name, value := range variables {
			expanded := variablePattern.ReplaceAllStringFunc(value, func(reference string) string {
				matches := variablePattern.FindStringSubmatch(reference)
				return variables[matches[1]]
			})
			if expanded != value {
				variables[name] = expanded
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return variables
}

func majorMinor(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return version
	}
	return strings.Join(parts[:2], ".")
}

func lineValue(content, prefix string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}
