package config

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"

	"github.com/pkg/errors"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema.json **/schema*.json
var schemaFs embed.FS

type schemaId struct {
	Id string `json:"$id"`
}

// compileSchema compiles schema bytes with jsonschema/v5. It loads all the referenced schema files that are
// referenced from the primary file.
func compileSchema() (*jsonschemav5.Schema, error) {
	c := jsonschemav5.NewCompiler()

	primaryId := ""

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

		var schemaId schemaId
		if err := json.Unmarshal(schemaBytes, &schemaId); err != nil {
			return errors.Wrapf(err, "failed to parse json for '%s'", path)
		}

		if path == "schema.json" {
			primaryId = schemaId.Id
		}

		return c.AddResource(schemaId.Id, bytes.NewReader(schemaBytes))
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to walk schema embed to load config schemas")
	}

	if primaryId == "" {
		return nil, errors.New("failed to find primary schema file")
	}

	schema, err := c.Compile(primaryId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to compile config schema for config schema validation")
	}

	return schema, nil
}
