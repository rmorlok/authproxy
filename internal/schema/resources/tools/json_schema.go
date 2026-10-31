package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rmorlok/authproxy/internal/schema/common"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	nativeSchemaDialect = "https://json-schema.org/draft/2020-12/schema"
	maxSchemaBytes      = 256 * 1024
	maxSchemaDepth      = 64
	schemaResourceURL   = "https://authproxy.invalid/tool-schema.json"
)

// CompileInputSchema compiles a required native JSON Schema 2020-12 document
// with an explicitly declared object root. References must resolve within the
// document; compilation never loads external resources. The returned validator
// owns its parsed source and may be reused concurrently if it is not mutated.
// Validation checks values without applying defaults, coercing types, or removing
// properties. Callers should decode JSON numbers with json.Decoder.UseNumber.
func CompileInputSchema(raw common.RawJSON) (*jsonschemav5.Schema, error) {
	return compileSchema(raw, true)
}

// CompileOutputSchema compiles a required native JSON Schema 2020-12 document
// that may accept any JSON value, including a boolean schema. An omitted output
// schema must be handled by the caller. Source ownership, local-reference policy,
// concurrent reuse, and read-only validation match CompileInputSchema.
func CompileOutputSchema(raw common.RawJSON) (*jsonschemav5.Schema, error) {
	return compileSchema(raw, false)
}

// validateSchema attaches the resource field's validation path to schema
// compilation errors without validating or changing invocation data.
func validateSchema(
	raw common.RawJSON,
	requireObject bool,
	vc *common.ValidationContext,
) error {
	if _, err := compileSchema(raw, requireObject); err != nil {
		return vc.NewError(err.Error())
	}
	return nil
}

// compileSchema applies the shared native-schema policy before retaining the
// compiled validator. Input roots explicitly declare object; a local root $ref
// may accompany that declaration. Both parsing passes own their decoded values.
func compileSchema(raw common.RawJSON, requireObject bool) (*jsonschemav5.Schema, error) {
	if len(raw) > maxSchemaBytes {
		return nil, fmt.Errorf("JSON schema exceeds %d bytes", maxSchemaBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %v", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("JSON schema must contain exactly one JSON value")
	}

	if !schemaJSONDepthAllowed(document, maxSchemaDepth) {
		return nil, fmt.Errorf("JSON schema nesting exceeds %d levels", maxSchemaDepth)
	}

	switch root := document.(type) {
	case map[string]any:
		if requireObject && !hasExplicitObjectType(root["type"]) {
			return nil, fmt.Errorf("input schema must declare top-level type object or [object]")
		}

		// Check the root before compilation: Compiler.Draft is a default, not
		// a restriction on an explicitly declared older dialect.
		if err := validateNativeSchemaDialect(root); err != nil {
			return nil, err
		}
	case bool:
		if requireObject {
			return nil, fmt.Errorf("input schema must declare top-level type object or [object]")
		}
	default:
		return nil, fmt.Errorf("JSON schema must be an object or boolean")
	}

	compiler := jsonschemav5.NewCompiler()
	compiler.Draft = jsonschemav5.Draft2020
	compiler.LoadURL = func(string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external JSON schema loading is disabled; bundle references in the schema")
	}

	compiler.RegisterExtension(
		"authproxy-native-dialect",
		nil, // meta
		nativeSchemaDialectCompiler{},
	)

	if err := compiler.AddResource(
		schemaResourceURL,
		bytes.NewReader(raw),
	); err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %v", err)
	}

	schema, err := compiler.Compile(schemaResourceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON schema: %v", err)
	}

	return schema, nil
}

// hasExplicitObjectType avoids trying to infer the root type through arbitrary
// compositions, references, or conditionals. A singleton type array is the
// equivalent JSON Schema spelling of type: object.
func hasExplicitObjectType(value any) bool {
	if value == "object" {
		return true
	}

	types, ok := value.([]any)
	return ok && len(types) == 1 && types[0] == "object"
}

// schemaJSONDepthAllowed bounds all authored JSON, including annotation data,
// without interpreting objects inside examples/default/const as schemas.
func schemaJSONDepthAllowed(value any, remaining int) bool {
	switch value := value.(type) {
	case map[string]any:
		if remaining == 0 {
			return false
		}
		for _, child := range value {
			if !schemaJSONDepthAllowed(child, remaining-1) {
				return false
			}
		}
	case []any:
		if remaining == 0 {
			return false
		}
		for _, child := range value {
			if !schemaJSONDepthAllowed(child, remaining-1) {
				return false
			}
		}
	}
	return true
}

// nativeSchemaDialectCompiler applies policy at the library's actual schema
// boundaries, including local references to nonstandard locations. Annotation
// values that merely contain $schema or $ref remain ordinary data. The library
// does not compile unused definitions; their dialect declarations are checked
// if a reference makes them part of the compiled schema.
type nativeSchemaDialectCompiler struct{}

// Compile checks the dialect at one compiler-selected schema location without
// adding runtime validation or interpreting annotation data.
func (nativeSchemaDialectCompiler) Compile(
	_ jsonschemav5.CompilerContext,
	schema map[string]any,
) (jsonschemav5.ExtSchema, error) {
	return nil, validateNativeSchemaDialect(schema)
}

// validateNativeSchemaDialect accepts an omitted declaration or the explicit
// native dialect, including its equivalent empty-fragment URI spelling.
func validateNativeSchemaDialect(schema map[string]any) error {
	if declaration, present := schema["$schema"]; present {
		dialect, ok := declaration.(string)
		if !ok || strings.TrimSuffix(dialect, "#") != nativeSchemaDialect {
			return fmt.Errorf("only JSON Schema 2020-12 is supported")
		}
	}
	return nil
}
