package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type profile struct {
	Runtime struct {
		Version    string `json:"version"`
		Entrypoint string `json:"entrypoint"`
	} `json:"runtime"`
	Database struct {
		MajorVersion         int    `json:"majorVersion"`
		MigrationToolVersion string `json:"migrationToolVersion"`
	} `json:"database"`
	Modules []string `json:"modules"`
	OpenAPI struct {
		Artifact   string `json:"artifact"`
		DigestFile string `json:"digestFile"`
		SHA256     string `json:"sha256"`
	} `json:"openapi"`
}

type goPackage struct {
	ImportPath string
	Imports    []string
}

func main() {
	root := "."
	if len(os.Args) == 2 {
		root = os.Args[1]
	} else if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: architecturecheck [repository-root]")
		os.Exit(2)
	}
	violations := validateRepository(root, listPackages)
	if len(violations) > 0 {
		for _, violation := range violations {
			fmt.Fprintln(os.Stderr, "- "+violation)
		}
		os.Exit(1)
	}
	fmt.Println("Architecture checks passed.")
}

func validateRepository(root string, packageLoader func(string) ([]goPackage, error)) []string {
	violations := []string{}
	profilePath := filepath.Join(root, "architecture-profile.json")
	schemaPath := filepath.Join(root, "architecture-profile.schema.json")
	profileDocument, rawProfile, err := readJSON(profilePath)
	if err != nil {
		return []string{err.Error()}
	}
	schemaDocument, _, err := readJSON(schemaPath)
	if err != nil {
		return []string{err.Error()}
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource("architecture-profile.schema.json", schemaDocument); err != nil {
		return []string{fmt.Sprintf("load architecture profile schema: %v", err)}
	}
	compiled, err := compiler.Compile("architecture-profile.schema.json")
	if err != nil {
		return []string{fmt.Sprintf("compile architecture profile schema: %v", err)}
	}
	if err := compiled.Validate(profileDocument); err != nil {
		violations = append(violations, fmt.Sprintf("architecture-profile.json does not satisfy schema: %v", err))
	}
	var current profile
	if err := json.Unmarshal(rawProfile, &current); err != nil {
		violations = append(violations, fmt.Sprintf("decode architecture profile: %v", err))
		return violations
	}

	artifact, err := os.ReadFile(filepath.Join(root, current.OpenAPI.Artifact))
	if err != nil {
		violations = append(violations, fmt.Sprintf("read OpenAPI artifact: %v", err))
	} else {
		digest := sha256.Sum256(artifact)
		actual := hex.EncodeToString(digest[:])
		if actual != current.OpenAPI.SHA256 {
			violations = append(violations, fmt.Sprintf("OpenAPI SHA-256 is %s but profile records %s", actual, current.OpenAPI.SHA256))
		}
		digestFile, readErr := os.ReadFile(filepath.Join(root, current.OpenAPI.DigestFile))
		if readErr != nil {
			violations = append(violations, fmt.Sprintf("read OpenAPI digest file: %v", readErr))
		} else if strings.TrimSpace(string(digestFile)) != actual {
			violations = append(violations, "OpenAPI digest file differs from the artifact")
		}
	}

	actualModules, err := directoryNames(filepath.Join(root, "internal/modules"))
	if err != nil {
		violations = append(violations, fmt.Sprintf("list modules: %v", err))
	} else if !equalStrings(actualModules, sortedCopy(current.Modules)) {
		violations = append(violations, fmt.Sprintf("profile modules %v differ from internal/modules %v", current.Modules, actualModules))
	}
	if info, err := os.Stat(filepath.Join(root, current.Runtime.Entrypoint)); err != nil || !info.IsDir() {
		violations = append(violations, "profile runtime entrypoint is not an existing directory")
	}
	goModule, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		violations = append(violations, fmt.Sprintf("read go.mod: %v", err))
	} else if !bytes.Contains(goModule, []byte("toolchain go"+current.Runtime.Version)) {
		violations = append(violations, "profile runtime version differs from go.mod toolchain")
	}
	dockerfile, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		violations = append(violations, fmt.Sprintf("read Dockerfile: %v", err))
	} else if !bytes.Contains(dockerfile, []byte("./"+current.Runtime.Entrypoint)) || !bytes.Contains(dockerfile, []byte(`ENTRYPOINT ["/api"]`)) {
		violations = append(violations, "Dockerfile build entrypoint or runtime command differs from profile")
	}
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		violations = append(violations, fmt.Sprintf("read Makefile: %v", err))
	} else if !bytes.Contains(makefile, []byte("MIGRATE_VERSION := "+current.Database.MigrationToolVersion)) {
		violations = append(violations, "profile migration tool version differs from Makefile")
	}
	compose, err := os.ReadFile(filepath.Join(root, "compose.yaml"))
	if err != nil {
		violations = append(violations, fmt.Sprintf("read compose.yaml: %v", err))
	} else if !bytes.Contains(compose, []byte(fmt.Sprintf("postgres:%d.", current.Database.MajorVersion))) {
		violations = append(violations, "profile PostgreSQL major version differs from compose.yaml")
	}
	httpServer, err := os.ReadFile(filepath.Join(root, "internal/platform/httpserver/server.go"))
	if err != nil {
		violations = append(violations, fmt.Sprintf("read HTTP server: %v", err))
	} else if !bytes.Contains(httpServer, []byte(`"dify-agent-api/v1"`)) {
		violations = append(violations, "HTTP version endpoint contract id differs from profile")
	}
	configSchema, err := os.ReadFile(filepath.Join(root, "deploy/config.schema.json"))
	if err != nil {
		violations = append(violations, fmt.Sprintf("read config schema: %v", err))
	} else {
		for _, releaseField := range []string{"DIFY_AGENT_RELEASE_ID", "DIFY_AGENT_BUILD_COMMIT"} {
			if !bytes.Contains(configSchema, []byte(`"`+releaseField+`"`)) {
				violations = append(violations, "config schema is missing runtime release field "+releaseField)
			}
		}
	}

	packages, err := packageLoader(root)
	if err != nil {
		violations = append(violations, fmt.Sprintf("list Go packages: %v", err))
	} else {
		for _, pkg := range packages {
			for _, imported := range pkg.Imports {
				if reason := forbiddenImport(pkg.ImportPath, imported); reason != "" {
					violations = append(violations, fmt.Sprintf("%s imports %s: %s", pkg.ImportPath, imported, reason))
				}
			}
		}
	}
	sort.Strings(violations)
	return violations
}

func forbiddenImport(source, target string) string {
	const marker = "/internal/modules/"
	const platformMarker = "/internal/platform/"
	const generatedMarker = "/generated/"
	sourceModule, sourceLayer, sourceOK := moduleLayer(source)
	targetModule, targetLayer, targetOK := moduleLayer(target)
	if strings.Contains(source, platformMarker) && strings.Contains(target, marker) {
		return "platform packages must not depend on business modules"
	}
	if !sourceOK {
		return ""
	}
	if targetOK && sourceModule != targetModule {
		if targetLayer != "application" || !isApplicationRoot(target) {
			return "cross-module imports must use the other module's public application contract"
		}
	}
	if sourceLayer == "domain" {
		if targetOK && (targetModule != sourceModule || targetLayer != "domain") {
			return "domain may only depend on its own domain packages"
		}
		if strings.Contains(target, platformMarker) || strings.Contains(target, generatedMarker) || isFrameworkImport(target) {
			return "domain must not depend on platform, generated code, HTTP, or PostgreSQL"
		}
	}
	if sourceLayer == "application" {
		if targetOK && targetModule == sourceModule && (targetLayer == "transport" || targetLayer == "infrastructure") {
			return "application must not depend on transport or infrastructure"
		}
		if strings.Contains(target, generatedMarker) || isFrameworkImport(target) {
			return "application must not depend on generated transport, HTTP, or PostgreSQL"
		}
	}
	if sourceLayer == "infrastructure" && targetOK && targetModule == sourceModule && targetLayer == "transport" {
		return "infrastructure must not depend on transport"
	}
	if sourceLayer == "transport" && targetOK && targetModule == sourceModule && targetLayer == "infrastructure" {
		return "transport must not depend on infrastructure"
	}
	return ""
}

func moduleLayer(importPath string) (string, string, bool) {
	const marker = "/internal/modules/"
	index := strings.Index(importPath, marker)
	if index < 0 {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(importPath[index:], marker), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func isApplicationRoot(importPath string) bool {
	_, layer, ok := moduleLayer(importPath)
	return ok && layer == "application" && strings.HasSuffix(importPath, "/application")
}

func isFrameworkImport(importPath string) bool {
	return strings.HasPrefix(importPath, "github.com/go-chi/") || strings.HasPrefix(importPath, "github.com/jackc/pgx/")
}

func listPackages(root string) ([]goPackage, error) {
	command := exec.Command("go", "list", "-json", "./...")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	packages := []goPackage{}
	for {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		packages = append(packages, pkg)
	}
	return packages, nil
}

func readJSON(path string) (any, []byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return document, content, nil
}

func directoryNames(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func equalStrings(left, right []string) bool {
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}
