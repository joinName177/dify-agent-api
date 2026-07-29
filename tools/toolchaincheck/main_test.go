package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRepository(t *testing.T) {
	t.Parallel()
	root := writeFixture(t)
	if violations := validate(root); len(violations) != 0 {
		t.Fatalf("validate() violations = %v", violations)
	}

	replaceFile(t, filepath.Join(root, "Dockerfile"), "golang:1.26.5-alpine@sha256:", "golang:1.25.1-alpine@sha256:")
	violations := validate(root)
	if !containsViolation(violations, "Dockerfile build image differs") {
		t.Fatalf("expected Dockerfile drift violation, got %v", violations)
	}
}

func TestValidateRejectsMutableActionAndDuplicateCIVersion(t *testing.T) {
	t.Parallel()
	root := writeFixture(t)
	path := filepath.Join(root, ".github/workflows/ci.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("\n  - uses: owner/action@v1\n  - run: staticcheck@v0.7.0\n")...)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	violations := validate(root)
	if !containsViolation(violations, "not pinned to a commit SHA") || !containsViolation(violations, "duplicate tool version") {
		t.Fatalf("expected action and duplicate version violations, got %v", violations)
	}
}

func TestParseMakeVariablesExpandsReferences(t *testing.T) {
	t.Parallel()
	variables := parseMakeVariables("VERSION := v1.2.3\nIMAGE := example/image:$(VERSION)@sha256:abc\n")
	if got := variables["IMAGE"]; got != "example/image:v1.2.3@sha256:abc" {
		t.Fatalf("IMAGE = %q", got)
	}
}

func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"Makefile": `GO_IMAGE := golang:1.26.5-alpine@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
RACE_GO_IMAGE := golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
OAPI_CODEGEN_VERSION := v2.6.0
MIGRATE_VERSION := v4.19.1
MIGRATE_IMAGE := migrate/migrate:$(MIGRATE_VERSION)@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
STATICCHECK_VERSION := v0.7.0
GOVULNCHECK_VERSION := v1.6.0
GITLEAKS_VERSION := v8.30.1
GITLEAKS_IMAGE := example/gitleaks:$(GITLEAKS_VERSION)@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
GOSEC_VERSION := v2.28.0
GO_LICENSES_VERSION := v2.0.1
GRYPE_VERSION := v0.116.0
GRYPE_IMAGE := example/grype:$(GRYPE_VERSION)@sha256:1111111111111111111111111111111111111111111111111111111111111111
generate:
	go run example/oapi-codegen@$(OAPI_CODEGEN_VERSION)
staticcheck:
	go run example/staticcheck@$(STATICCHECK_VERSION)
govulncheck:
	go run example/govulncheck@$(GOVULNCHECK_VERSION)
migration-apply-ci:
	go run example/migrate@$(MIGRATE_VERSION)
staged-check:
	GITLEAKS_IMAGE='$(GITLEAKS_IMAGE)' ./staged-check.sh
gosec:
	go run example/gosec@$(GOSEC_VERSION)
license-report:
	go run example/go-licenses/v2@$(GO_LICENSES_VERSION)
container-security-check:
	GRYPE_IMAGE='$(GRYPE_IMAGE)' ./container-security-check.sh
`,
		"go.mod":    "module example.test/toolchain\n\ngo 1.26.0\n\ntoolchain go1.26.5\n",
		"README.md": "The authoritative version is `toolchain go1.26.5`.\n",
		"Dockerfile": `# syntax=docker/dockerfile:1.7@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee
FROM golang:1.26.5-alpine@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa AS build
FROM example/runtime@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff
`,
		".github/workflows/ci.yaml": `jobs:
  governance-check:
    name: governance-check
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make ci-governance-check GO_RUNNER=
  format-check:
    name: format-check
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make format-check GO_RUNNER=
  generated-code-drift:
    name: generated-code-drift
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make generated-check OAPI_CODEGEN_RUNTIME=local
  module-check:
    name: module-check
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make mod-check GO_RUNNER=
  static-analysis:
    name: static-analysis
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make static-analysis GO_RUNNER=
  unit-race-test:
    name: unit-race-test
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make unit-race-test GO_RUNNER= RACE_RUNNER=
  architecture-check:
    name: architecture-check
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make architecture-check GO_RUNNER=
  migration-check:
    name: migration-check
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make migration-check GO_RUNNER=
  integration-test:
    name: integration-test
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make integration-test GO_RUNNER=
  e2e-test:
    name: e2e-test
    container: golang:1.26.5-bookworm@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - uses: ./.github/actions/go-cache
      - run: make e2e-test GO_RUNNER=
  container-build:
    name: container-build
    steps:
      - uses: actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd
      - run: make container-build IMAGE_TAG=dify-agent-api:ci
`,
		".github/actions/go-cache/action.yml": `runs:
  using: composite
  steps:
    - uses: actions/cache@27d5ce7f107fe9357f9df03efb73ab90386fccae
      with:
        path: |
          /go/pkg/mod
          /root/.cache/go-build
        key: ${{ runner.os }}-${{ runner.arch }}-go-${{ hashFiles('go.sum', 'Makefile') }}
`,
		"api/openapi/v1/oapi-codegen.yaml": "# schema=https://example.test/oapi-codegen/v2.6.0/configuration-schema.json\n",
	}
	for path, content := range files {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func replaceFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(content), old, replacement, 1)
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsViolation(violations []string, fragment string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, fragment) {
			return true
		}
	}
	return false
}
