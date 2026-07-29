package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRepositoryMigrationManifest(t *testing.T) {
	if err := validateFiles("../../migrations", "../../migrations/manifest.json"); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsPublishedMigrationMutation(t *testing.T) {
	manifestBytes, files := repositoryInputs(t)
	files["000001_bootstrap.up.sql"] = append(files["000001_bootstrap.up.sql"], []byte("\nSELECT 1;\n")...)
	if err := validate(manifestBytes, files); err == nil || !strings.Contains(err.Error(), "checksum changed") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
}

func TestRejectsUnrecordedMigration(t *testing.T) {
	manifestBytes, files := repositoryInputs(t)
	files["000003_unrecorded.up.sql"] = []byte("SELECT 1;")
	if err := validate(manifestBytes, files); err == nil || !strings.Contains(err.Error(), "not recorded") {
		t.Fatalf("expected manifest failure, got %v", err)
	}
}

func TestRejectsMissingPairAndVersionGap(t *testing.T) {
	manifestBytes, files := repositoryInputs(t)
	delete(files, "000002_users_and_sessions.down.sql")
	if err := validate(manifestBytes, files); err == nil || !strings.Contains(err.Error(), "missing migrations") {
		t.Fatalf("expected missing down migration failure, got %v", err)
	}

	var baseline manifest
	if err := json.Unmarshal(manifestBytes, &baseline); err != nil {
		t.Fatal(err)
	}
	baseline.Migrations[1].Version = 3
	broken, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(broken, files); err == nil || !strings.Contains(err.Error(), "contiguous") {
		t.Fatalf("expected version gap failure, got %v", err)
	}
}

func TestGeneratedVersionUsesManifestLatestVersion(t *testing.T) {
	manifestBytes, _ := repositoryInputs(t)
	version, err := latestVersion(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 || !strings.Contains(string(generatedVersionSource(version)), "ExpectedVersion uint = 2") {
		t.Fatalf("unexpected generated migration version %d", version)
	}
}

func TestRejectsStaleGeneratedVersion(t *testing.T) {
	if err := validateGeneratedVersion(generatedVersionSource(2), generatedVersionSource(1)); err == nil {
		t.Fatal("expected stale generated migration version to fail")
	}
}

func repositoryInputs(t *testing.T) ([]byte, map[string][]byte) {
	t.Helper()
	manifestBytes, err := os.ReadFile("../../migrations/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	files := make(map[string][]byte)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			contents, err := os.ReadFile("../../migrations/" + entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name()] = contents
		}
	}
	return manifestBytes, files
}
