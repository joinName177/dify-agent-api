package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryConfigurationContract(t *testing.T) {
	if err := validateFiles("../../deploy/config.schema.json", "../../internal/platform/config/config.go"); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsRuntimeKeyDrift(t *testing.T) {
	schema, source := repositoryInputs(t)
	source = []byte(strings.Replace(string(source), `"DIFY_AGENT_RATE_LIMIT_BURST": {},`, "", 1))
	if err := validate(schema, source); err == nil || !strings.Contains(err.Error(), "configuration key mismatch") {
		t.Fatalf("expected key drift error, got %v", err)
	}
}

func TestRejectsDefaultDrift(t *testing.T) {
	schema, source := repositoryInputs(t)
	schema = []byte(strings.Replace(string(schema), `"default": "10"`, `"default": "11"`, 1))
	if err := validate(schema, source); err == nil || !strings.Contains(err.Error(), "default mismatch") {
		t.Fatalf("expected default drift error, got %v", err)
	}
}

func TestRejectsUnknownSchemaDraft(t *testing.T) {
	schema, source := repositoryInputs(t)
	schema = []byte(strings.Replace(string(schema), "draft/2020-12", "draft/2019-09", 1))
	if err := validate(schema, source); err == nil || !strings.Contains(err.Error(), "Draft 2020-12") {
		t.Fatalf("expected schema draft error, got %v", err)
	}
}

func TestRejectsNonStringEnvironmentValueType(t *testing.T) {
	schema, source := repositoryInputs(t)
	schema = []byte(strings.Replace(string(schema), `"DIFY_AGENT_DATABASE_MAX_CONNS": { "type": "string"`, `"DIFY_AGENT_DATABASE_MAX_CONNS": { "type": "integer"`, 1))
	if err := validate(schema, source); err == nil || !strings.Contains(err.Error(), "string type") {
		t.Fatalf("expected environment type error, got %v", err)
	}
}

func TestRejectsAllowedButUnreadRuntimeKey(t *testing.T) {
	schema, source := repositoryInputs(t)
	source = []byte(strings.Replace(string(source), `values["DIFY_AGENT_OTEL_EXPORTER_OTLP_ENDPOINT"]`, `values["DIFY_AGENT_RELEASE_ID"]`, 1))
	if err := validate(schema, source); err == nil || !strings.Contains(err.Error(), "not read") {
		t.Fatalf("expected unread key error, got %v", err)
	}
}

func TestMissingInputFailsClosed(t *testing.T) {
	if err := validateFiles(filepath.Join(t.TempDir(), "missing.json"), "missing.go"); err == nil {
		t.Fatal("expected missing input to fail")
	}
}

func repositoryInputs(t *testing.T) ([]byte, []byte) {
	t.Helper()
	schema, err := os.ReadFile("../../deploy/config.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("../../internal/platform/config/config.go")
	if err != nil {
		t.Fatal(err)
	}
	return schema, source
}
