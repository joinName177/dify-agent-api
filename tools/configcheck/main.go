package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
)

const (
	schemaPath = "deploy/config.schema.json"
	sourcePath = "internal/platform/config/config.go"
)

type schemaDocument struct {
	Schema               string                    `json:"$schema"`
	AdditionalProperties bool                      `json:"additionalProperties"`
	Required             []string                  `json:"required"`
	Properties           map[string]schemaProperty `json:"properties"`
}

type schemaProperty struct {
	Type    string `json:"type"`
	Default any    `json:"default"`
}

type runtimeContract struct {
	Allowed  []string
	Required []string
	Defaults map[string]string
	Reads    map[string]struct{}
}

func main() {
	if err := validateFiles(schemaPath, sourcePath); err != nil {
		fmt.Fprintf(os.Stderr, "config-check: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("config-check: configuration schema and runtime contract are consistent")
}

func validateFiles(schemaFile, sourceFile string) error {
	schemaBytes, err := os.ReadFile(schemaFile)
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	sourceBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("read runtime source: %w", err)
	}
	return validate(schemaBytes, sourceBytes)
}

func validate(schemaBytes, sourceBytes []byte) error {
	var schema schemaDocument
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	if schema.Schema != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("schema must use JSON Schema Draft 2020-12")
	}
	if schema.AdditionalProperties {
		return errors.New("schema must reject unknown configuration keys")
	}

	runtime, err := parseRuntimeContract(sourceBytes)
	if err != nil {
		return err
	}
	properties := make([]string, 0, len(schema.Properties))
	for key := range schema.Properties {
		properties = append(properties, key)
		if schema.Properties[key].Type != "string" {
			return fmt.Errorf("environment variable %s must use JSON Schema string type", key)
		}
	}
	if err := compareSets("schema properties", properties, "runtime allowed keys", runtime.Allowed); err != nil {
		return err
	}
	if err := compareSets("schema required keys", schema.Required, "runtime required keys", runtime.Required); err != nil {
		return err
	}

	for key, property := range schema.Properties {
		schemaDefault, hasDefault, err := normalizedDefault(property.Default)
		if err != nil {
			return fmt.Errorf("%s default: %w", key, err)
		}
		runtimeDefault, runtimeHasDefault := runtime.Defaults[key]
		if hasDefault != runtimeHasDefault || (hasDefault && schemaDefault != runtimeDefault) {
			return fmt.Errorf("default mismatch for %s: schema=%q runtime=%q", key, schemaDefault, runtimeDefault)
		}
	}
	for key := range runtime.Defaults {
		if _, ok := schema.Properties[key]; !ok {
			return fmt.Errorf("runtime default exists for unknown key %s", key)
		}
	}
	for _, key := range runtime.Allowed {
		if _, ok := runtime.Reads[key]; !ok {
			return fmt.Errorf("allowed configuration key %s is not read by runtime code", key)
		}
	}
	return nil
}

func parseRuntimeContract(source []byte) (runtimeContract, error) {
	file, err := parser.ParseFile(token.NewFileSet(), sourcePath, source, 0)
	if err != nil {
		return runtimeContract{}, fmt.Errorf("parse runtime source: %w", err)
	}
	contract := runtimeContract{Defaults: make(map[string]string), Reads: make(map[string]struct{})}
	foundAllowed := false
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.ValueSpec:
			for index, name := range typed.Names {
				if name.Name != "allowed" || index >= len(typed.Values) {
					continue
				}
				composite, ok := typed.Values[index].(*ast.CompositeLit)
				if !ok {
					continue
				}
				foundAllowed = true
				for _, element := range composite.Elts {
					keyValue, ok := element.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if key, ok := stringLiteral(keyValue.Key); ok {
						contract.Allowed = append(contract.Allowed, key)
					}
				}
			}
		case *ast.CallExpr:
			identifier, ok := typed.Fun.(*ast.Ident)
			if !ok || len(typed.Args) == 0 {
				break
			}
			key, ok := stringLiteral(typed.Args[0])
			if !ok {
				break
			}
			switch identifier.Name {
			case "required":
				contract.Required = append(contract.Required, key)
				contract.Reads[key] = struct{}{}
			case "withDefault":
				if len(typed.Args) == 2 {
					if fallback, ok := stringLiteral(typed.Args[1]); ok {
						contract.Defaults[key] = fallback
						contract.Reads[key] = struct{}{}
					}
				}
			}
		case *ast.IndexExpr:
			identifier, ok := typed.X.(*ast.Ident)
			if !ok || identifier.Name != "values" {
				break
			}
			if key, ok := stringLiteral(typed.Index); ok {
				contract.Reads[key] = struct{}{}
			}
		}
		return true
	})
	if !foundAllowed {
		return runtimeContract{}, errors.New("runtime allowed-key map was not found")
	}
	return contract, nil
}

func stringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func compareSets(leftName string, left []string, rightName string, right []string) error {
	leftSet, rightSet := stringSet(left), stringSet(right)
	var onlyLeft, onlyRight []string
	for value := range leftSet {
		if _, ok := rightSet[value]; !ok {
			onlyLeft = append(onlyLeft, value)
		}
	}
	for value := range rightSet {
		if _, ok := leftSet[value]; !ok {
			onlyRight = append(onlyRight, value)
		}
	}
	if len(onlyLeft) == 0 && len(onlyRight) == 0 {
		return nil
	}
	sort.Strings(onlyLeft)
	sort.Strings(onlyRight)
	return fmt.Errorf("configuration key mismatch: only in %s=%v; only in %s=%v", leftName, onlyLeft, rightName, onlyRight)
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func normalizedDefault(value any) (string, bool, error) {
	if value == nil {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("environment defaults must be strings, got %T", value)
	}
	return text, true, nil
}
