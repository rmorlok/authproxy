package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type ConfiguredActorsExternalSource struct {
	KeysPath    string               `json:"keysPath" yaml:"keysPath"`
	Permissions []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

func (s *ConfiguredActorsExternalSource) AllInNamespace(namespace string) []*ConfiguredActor {
	entries, err := os.ReadDir(s.KeysPath)
	if err != nil {
		panic(err)
	}

	actors := make([]*ConfiguredActor, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			// Skip directories
			continue
		}

		// Check if the file has the desired extension
		if strings.HasSuffix(entry.Name(), ".pub") {
			externalId := strings.TrimSuffix(entry.Name(), ".pub")
			actors = append(actors, &ConfiguredActor{
				ExternalId: externalId,
				Namespace:  namespace,
				Key: &Key{
					InnerVal: &KeyPublicPrivate{
						PublicKey: &KeyData{
							InnerVal: &KeyDataFile{
								Path: filepath.Join(s.KeysPath, entry.Name()),
							},
						},
					},
				},
				Permissions: slices.Clone(s.Permissions),
			})
		}
	}

	return actors
}
