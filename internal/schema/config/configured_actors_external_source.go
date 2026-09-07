package config

import (
	"os"
	"path/filepath"
	"strings"

	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

type ConfiguredActorsExternalSource struct {
	KeysPath    string               `json:"keysPath" yaml:"keysPath"`
	Permissions []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
}

func (s *ConfiguredActorsExternalSource) AllInNamespace(namespace string) []*actorschema.Actor {
	entries, err := os.ReadDir(s.KeysPath)
	if err != nil {
		panic(err)
	}

	actors := make([]*actorschema.Actor, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			// Skip directories
			continue
		}

		// Check if the file has the desired extension
		if strings.HasSuffix(entry.Name(), ".pub") {
			externalId := strings.TrimSuffix(entry.Name(), ".pub")
			actors = append(actors, &actorschema.Actor{
				TypeMeta: meta.NewTypeMeta(actorschema.ActorKind),
				Metadata: meta.ObjectMeta{Namespace: namespace},
				Spec: actorschema.ActorSpec{
					ExternalId: externalId,
					SigningKey: &keyschema.SigningKey{
						InnerVal: &keyschema.KeyPublicPrivate{
							PublicKey: &keyschema.KeyData{
								InnerVal: &keyschema.KeyDataFile{
									Path: filepath.Join(s.KeysPath, entry.Name()),
								},
							},
						},
					},
					Permissions: actorschema.ClonePermissions(s.Permissions),
				},
			})
		}
	}

	return actors
}
