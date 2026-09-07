// Command check-contract-casing prevents AuthProxy-owned wire contracts from
// drifting away from lowerCamelCase. It intentionally excludes opaque provider
// payloads, user-authored JSON Schema/UI Schema content, database columns,
// metric/permission/enum values, labels, headers, CLI flags, and environment
// variables: those are identifiers rather than AuthProxy object fields.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var lowerCamelCase = regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)

// These tags belong to an external provider API rather than AuthProxy's wire
// contract. Keep the exception exact so other seed-service fields remain
// covered by the lowerCamelCase guard.
var externalJSONTags = map[string]map[string]struct{}{
	"demos/seed/backend/main.go": {
		"display_name":               {},
		"header_name":                {},
		"redirect_uri":               {},
		"require_pkce":               {},
		"required_scope":             {},
		"token_endpoint_auth_method": {},
	},
}

var goContractDirs = []string{
	"cmd/cli", "cmd/loadtest", "demos/seed/backend", "demos/shell/backend",
	"internal/apauth", "internal/app_metrics", "internal/apredis", "internal/config",
	"internal/core", "internal/httperr", "internal/loadtest", "internal/routes",
	"internal/schema", "internal/tasks", "plugins/grafana/authproxy-datasource/pkg/plugin",
	"terraform/provider/internal/client",
}

var firstPartyDirs = []string{
	"sdks/js/src", "ui/admin/src", "ui/marketplace/src",
	"plugins/grafana/authproxy-datasource/src",
}

var legacyWireTerms = []string{
	"auth_token", "return_to", "return_to_url", "_proxy_raw", "_force_state",
	"_disconnect_all", "_migrate_version", "_setup_step", "_data_source", "_cancel_setup",
	"body_json", "body_raw", "connection_id", "external_id",
	"into_namespace", "label_selector", "resource_type", "task_id", "target_version",
}

func main() {
	var violations []string
	violations = append(violations, checkGoTags()...)
	violations = append(violations, checkSchemas()...)
	violations = append(violations, checkSwagger()...)
	violations = append(violations, checkFirstPartyWireLiterals()...)

	if len(violations) == 0 {
		fmt.Println("contract casing guard passed")
		return
	}

	sort.Strings(violations)
	for _, violation := range violations {
		fmt.Fprintln(os.Stderr, violation)
	}
	os.Exit(1)
}

func checkGoTags() []string {
	var violations []string
	for _, dir := range goContractDirs {
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if parseErr != nil {
				violations = append(violations, fmt.Sprintf("%s: parse Go tags: %v", path, parseErr))
				return nil
			}
			ast.Inspect(file, func(node ast.Node) bool {
				field, ok := node.(*ast.Field)
				if !ok || field.Tag == nil {
					return true
				}
				tagText, unquoteErr := strconv.Unquote(field.Tag.Value)
				if unquoteErr != nil {
					return true
				}
				tag := reflect.StructTag(tagText)
				for _, kind := range []string{"json", "yaml", "form"} {
					name := strings.Split(tag.Get(kind), ",")[0]
					if name != "" && name != "-" && name != "$id" && !lowerCamelCase.MatchString(name) &&
						!(kind == "json" && isExternalJSONTag(path, name)) {
						violations = append(violations, fmt.Sprintf("%s: %s tag %q must be lowerCamelCase", path, kind, name))
					}
				}
				return true
			})
			return nil
		})
	}
	return violations
}

func isExternalJSONTag(path, name string) bool {
	allowed, ok := externalJSONTags[filepath.ToSlash(path)]
	if !ok {
		return false
	}
	_, ok = allowed[name]
	return ok
}

func checkSchemas() []string {
	var violations []string
	violations = append(violations, checkSchemaFile("cmd/cli/config/schema.json")...)
	_ = filepath.WalkDir("internal/schema", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasPrefix(entry.Name(), "schema") || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		// These schemas describe opaque user-authored documents. Their property
		// names are deliberately not AuthProxy-owned contract fields.
		if path == "internal/schema/common/json_schema/schema.json" || path == "internal/schema/common/ui_schema/schema.json" {
			return nil
		}
		violations = append(violations, checkSchemaFile(path)...)
		return nil
	})
	return violations
}

func checkSchemaFile(path string) []string {
	var document any
	if err := decodeJSONFile(path, &document); err != nil {
		return []string{fmt.Sprintf("%s: parse JSON Schema: %v", path, err)}
	}
	return inspectSchema(document, path)
}

func inspectSchema(value any, location string) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	var violations []string
	for _, key := range []string{"properties", "dependentRequired"} {
		if fields, ok := object[key].(map[string]any); ok {
			for name := range fields {
				if !lowerCamelCase.MatchString(name) {
					violations = append(violations, fmt.Sprintf("%s: JSON Schema %s field %q must be lowerCamelCase", location, key, name))
				}
			}
		}
	}
	if required, ok := object["required"].([]any); ok {
		for _, field := range required {
			if name, ok := field.(string); ok && !lowerCamelCase.MatchString(name) {
				violations = append(violations, fmt.Sprintf("%s: JSON Schema required field %q must be lowerCamelCase", location, name))
			}
		}
	}
	for _, child := range object {
		violations = append(violations, inspectSchema(child, location)...)
	}
	return violations
}

func checkSwagger() []string {
	files := []string{
		"internal/service/api/swagger/docs.json",
		"internal/service/admin_api/swagger/docs.json",
	}
	var violations []string
	for _, path := range files {
		var document map[string]any
		if err := decodeJSONFile(path, &document); err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse Swagger: %v", path, err))
			continue
		}
		if definitions, ok := document["definitions"].(map[string]any); ok {
			for name, definition := range definitions {
				violations = append(violations, inspectSwaggerDefinition(definition, path+" definition "+name)...)
			}
		}
		if paths, ok := document["paths"].(map[string]any); ok {
			for route, operations := range paths {
				for _, segment := range strings.Split(route, "/") {
					if strings.HasPrefix(segment, "_") && !lowerCamelCase.MatchString(strings.TrimPrefix(segment, "_")) {
						violations = append(violations, fmt.Sprintf("%s: Swagger action route %q must retain only a leading underscore", path, route))
					}
					if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") && !lowerCamelCase.MatchString(strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")) {
						violations = append(violations, fmt.Sprintf("%s: Swagger path parameter %q must be lowerCamelCase", path, segment))
					}
				}
				violations = append(violations, inspectSwaggerParameters(operations, path+" route "+route)...)
			}
		}
	}
	return violations
}

func inspectSwaggerDefinition(value any, location string) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	var violations []string
	if properties, ok := object["properties"].(map[string]any); ok {
		for name := range properties {
			if !lowerCamelCase.MatchString(name) {
				violations = append(violations, fmt.Sprintf("%s: Swagger property %q must be lowerCamelCase", location, name))
			}
		}
	}
	return violations
}

func inspectSwaggerParameters(value any, location string) []string {
	object, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	var violations []string
	for _, operationValue := range object {
		operation, ok := operationValue.(map[string]any)
		if !ok {
			continue
		}
		parameters, _ := operation["parameters"].([]any)
		for _, parameterValue := range parameters {
			parameter, ok := parameterValue.(map[string]any)
			if !ok {
				continue
			}
			in, _ := parameter["in"].(string)
			name, _ := parameter["name"].(string)
			if (in == "query" || in == "path" || in == "formData") && !lowerCamelCase.MatchString(name) {
				violations = append(violations, fmt.Sprintf("%s: Swagger %s parameter %q must be lowerCamelCase", location, in, name))
			}
		}
	}
	return violations
}

func checkFirstPartyWireLiterals() []string {
	var violations []string
	for _, dir := range firstPartyDirs {
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			for _, term := range legacyWireTerms {
				if strings.Contains(string(content), term) {
					violations = append(violations, fmt.Sprintf("%s: contains legacy AuthProxy wire name %q", path, term))
				}
			}
			return nil
		})
	}
	return violations
}

func decodeJSONFile(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, destination)
}
