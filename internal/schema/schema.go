package schema

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"sync"

	"github.com/pkg/errors"
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
				return errors.Wrapf(err, "failed to read schema file '%s'", path)
			}

			var schemaId schemaIdStruct
			if err := json.Unmarshal(schemaBytes, &schemaId); err != nil {
				return errors.Wrapf(err, "failed to parse json for '%s'", path)
			}

			return schemaCompiler.AddResource(schemaId.Id, bytes.NewReader(schemaBytes))
		})

		if err != nil {
			schemaErr = errors.Wrap(err, "failed to walk schema embed to load config schemas")
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
		return nil, errors.Wrapf(err, "failed to compile schema '%s'", schemaId)
	}

	compileMutex.Lock()
	defer compileMutex.Unlock()
	schemaCache[schemaId] = compiled

	return schemaCache[schemaId], nil
}
