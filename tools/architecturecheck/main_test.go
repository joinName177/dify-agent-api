package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestForbiddenImportMatrix(t *testing.T) {
	t.Parallel()
	module := "example.test/service/internal/modules/user/"
	tests := []struct {
		name, source, target string
		forbidden            bool
	}{
		{"domain own domain", module + "domain", module + "domain/value", false},
		{"domain application", module + "domain", module + "application/ports", true},
		{"domain platform", module + "domain", "example.test/service/internal/platform/config", true},
		{"domain pgx", module + "domain", "github.com/jackc/pgx/v5", true},
		{"application domain", module + "application/usecases", module + "domain", false},
		{"application infrastructure", module + "application/usecases", module + "infrastructure/postgres", true},
		{"application generated", module + "application/usecases", "example.test/service/generated/openapi/v1", true},
		{"infrastructure ports", module + "infrastructure/postgres", module + "application/ports", false},
		{"infrastructure transport", module + "infrastructure/postgres", module + "transport/http", true},
		{"transport application", module + "transport/http", module + "application/usecases", false},
		{"transport infrastructure", module + "transport/http", module + "infrastructure/postgres", true},
		{"platform module", "example.test/service/internal/platform/auth", module + "domain", true},
		{"cross module deep", module + "application/usecases", "example.test/service/internal/modules/billing/domain", true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := forbiddenImport(test.source, test.target) != ""
			if got != test.forbidden {
				t.Fatalf("forbiddenImport(%q, %q) forbidden = %v, reason = %q", test.source, test.target, got, forbiddenImport(test.source, test.target))
			}
		})
	}
}

func TestValidateRepositoryRejectsDigestAndModuleDrift(t *testing.T) {
	root := copyArchitectureFixture(t)
	profilePath := filepath.Join(root, "architecture-profile.json")
	content, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "49184e9d62e836a155fd2f6ed555107cfdb657f8f2dd5a7dd6dc7cd36b56ae2d", strings.Repeat("0", 64), 1))
	if err := os.WriteFile(profilePath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal/modules/extra"), 0o700); err != nil {
		t.Fatal(err)
	}
	violations := validateRepository(root, func(string) ([]goPackage, error) { return nil, nil })
	if !containsArchitectureViolation(violations, "OpenAPI SHA-256") || !containsArchitectureViolation(violations, "profile modules") {
		t.Fatalf("expected digest and module violations, got %v", violations)
	}
}

func TestSchemaRejectsUnknownProperty(t *testing.T) {
	root := copyArchitectureFixture(t)
	path := filepath.Join(root, "architecture-profile.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), `"project": "dify-agent-api",`, `"project": "dify-agent-api", "unexpected": true,`, 1))
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	violations := validateRepository(root, func(string) ([]goPackage, error) { return nil, nil })
	if !containsArchitectureViolation(violations, "does not satisfy schema") {
		t.Fatalf("expected schema violation, got %v", violations)
	}
}

func TestSchemaRejectsAuthenticationConflict(t *testing.T) {
	root := copyArchitectureFixture(t)
	path := filepath.Join(root, "architecture-profile.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), `"passwordHash": "argon2id"`, `"passwordHash": "bcrypt"`, 1))
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	violations := validateRepository(root, func(string) ([]goPackage, error) { return nil, nil })
	if !containsArchitectureViolation(violations, "does not satisfy schema") {
		t.Fatalf("expected authentication schema violation, got %v", violations)
	}
}

func copyArchitectureFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repositoryRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"architecture-profile.json", "architecture-profile.schema.json", "api/openapi/v1/openapi.yaml", "api/openapi/v1/openapi.sha256", "deploy/config.schema.json", "go.mod", "Dockerfile", "Makefile", "compose.yaml", "internal/platform/httpserver/server.go"} {
		content, err := os.ReadFile(filepath.Join(repositoryRoot, path))
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "internal/modules/user"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "cmd/api"), 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func containsArchitectureViolation(violations []string, fragment string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, fragment) {
			return true
		}
	}
	return false
}
