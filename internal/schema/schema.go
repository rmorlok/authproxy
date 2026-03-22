package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed **/schema*.json
var schemaFs embed.FS

type schemaIdStruct struct {
	Id string `json:"$id"`
}

var schemaOnce sync.Once
var schemaCompiler *jsonschemav5.Compiler
var schemaErr error
var schemaCache = make(map[string]*jsonschemav5.Schema)
var compileMutex sync.RWMutex

func loadSchemasOnce() error {
	schemaOnce.Do(func() {
		schemaCompiler = jsonschemav5.NewCompiler()

		err := fs.WalkDir(schemaFs, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			schemaBytes, err := schemaFs.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read schema file '%s': %w", path, err)
			}

			var schemaId schemaIdStruct
			if err := json.Unmarshal(schemaBytes, &schemaId); err != nil {
				return fmt.Errorf("failed to parse json for '%s': %w", path, err)
			}

			return schemaCompiler.AddResource(schemaId.Id, bytes.NewReader(schemaBytes))
		})

		if err != nil {
			schemaErr = fmt.Errorf("failed to walk schema embed to load config schemas: %w", err)
		}
	})

	return schemaErr
}

// CompileSchema compiles schema bytes with jsonschema/v5. It loads all the referenced schema files that are
// referenced from the primary file.
func CompileSchema(schemaId string) (*jsonschemav5.Schema, error) {
	if err := loadSchemasOnce(); err != nil {
		return nil, err
	}

	var s *jsonschemav5.Schema
	var ok bool
	compileMutex.RLock()
	s, ok = schemaCache[schemaId]
	compileMutex.RUnlock()

	if ok {
		return s, nil
	}

	compiled, err := schemaCompiler.Compile(schemaId)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema '%s': %w", schemaId, err)
	}

	compileMutex.Lock()
	defer compileMutex.Unlock()
	schemaCache[schemaId] = compiled

	return schemaCache[schemaId], nil
}
