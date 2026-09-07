package config

import (
	"testing"

	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSystemAuth(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("namespaced actor path", func(t *testing.T) {
			data := `
  cookieDomain: localhost:8080
  jwtSigningKey:
    publicKey:
      path: ./dev_config/keys/system.pub
    privateKey:
      path: ./dev_config/keys/system
  actors:
    root:
      keysPath: ./dev_config/keys/actors
`
			expected := SystemAuth{
				JwtSigningKey: &Key{
					InnerVal: &KeyPublicPrivate{
						PublicKey: &KeyData{
							InnerVal: &KeyDataFile{
								Path: "./dev_config/keys/system.pub",
							},
						},
						PrivateKey: &KeyData{
							InnerVal: &KeyDataFile{
								Path: "./dev_config/keys/system",
							},
						},
					},
				},
				Actors: &ConfiguredActors{
					InnerVal: &ConfiguredActorsExternalSources{
						Sources: map[string]*ConfiguredActorsExternalSource{
							"root": {KeysPath: "./dev_config/keys/actors"},
						},
					},
				},
			}

			var sa SystemAuth
			err := yaml.Unmarshal([]byte(data), &sa)
			assert.NoError(err)
			assert.Equal(expected, sa)
		})
		t.Run("rejects legacy actor path", func(t *testing.T) {
			data := `
actors:
  keysPath: ./dev_config/keys/actors
`
			var sa SystemAuth
			err := yaml.Unmarshal([]byte(data), &sa)
			assert.ErrorContains(err, "invalid actor source namespace")
		})
		t.Run("namespaced actor paths", func(t *testing.T) {
			data := `
cookieDomain: localhost:8080
jwtSigningKey:
  publicKey:
    path: ./dev_config/keys/system.pub
  privateKey:
    path: ./dev_config/keys/system
actors:
  root:
    keysPath: ./dev_config/keys/actors/root
  root.smoke:
    keysPath: ./dev_config/keys/actors/smoke
    permissions:
      - namespace: root.smoke.{{external_id}}
        resources: [connections]
        verbs: [create]
  syncCronSchedule: "* * * * *"
`
			var sa SystemAuth
			err := yaml.Unmarshal([]byte(data), &sa)
			assert.NoError(err)

			sources, ok := sa.Actors.InnerVal.(*ConfiguredActorsExternalSources)
			assert.True(ok)
			assert.Equal("* * * * *", sources.SyncCronSchedule)
			assert.Equal("./dev_config/keys/actors/root", sources.Sources["root"].KeysPath)
			assert.Equal("./dev_config/keys/actors/smoke", sources.Sources["root.smoke"].KeysPath)
			assert.Equal("root.smoke.{{external_id}}", sources.Sources["root.smoke"].Permissions[0].Namespace)
		})
		t.Run("actors list", func(t *testing.T) {
			data := `
cookieDomain: localhost:8080
jwtSigningKey:
  publicKey:
    path: ./dev_config/keys/system.pub
  privateKey:
    path: ./dev_config/keys/system
actors:
  - apiVersion: authproxy.net/v1alpha1
    kind: Actor
    metadata:
      namespace: root.admins
    spec:
      externalId: bobdole
      signingKey:
        publicKey:
          path: ./dev_config/keys/actors/bobdole.pub
`
			expected := SystemAuth{
				JwtSigningKey: &Key{
					InnerVal: &KeyPublicPrivate{
						PublicKey: &KeyData{
							InnerVal: &KeyDataFile{
								Path: "./dev_config/keys/system.pub",
							},
						},
						PrivateKey: &KeyData{
							InnerVal: &KeyDataFile{
								Path: "./dev_config/keys/system",
							},
						},
					},
				},
				Actors: &ConfiguredActors{
					InnerVal: ConfiguredActorsList{
						&actorschema.Actor{
							TypeMeta: meta.NewTypeMeta(actorschema.ActorKind),
							Metadata: meta.ObjectMeta{Namespace: "root.admins"},
							Spec: actorschema.ActorSpec{
								ExternalId: "bobdole",
								SigningKey: &Key{
									InnerVal: &KeyPublicPrivate{
										PublicKey: &KeyData{
											InnerVal: &KeyDataFile{
												Path: "./dev_config/keys/actors/bobdole.pub",
											},
										},
									},
								},
							},
						},
					},
				},
			}

			var sa SystemAuth
			err := yaml.Unmarshal([]byte(data), &sa)
			assert.NoError(err)
			assert.Equal(expected, sa)
		})
	})
}
