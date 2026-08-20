package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSystemAuth(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("actors path", func(t *testing.T) {
			data := `
  cookieDomain: localhost:8080
  jwtSigningKey:
    publicKey:
      path: ./dev_config/keys/system.pub
    privateKey:
      path: ./dev_config/keys/system
  actors:
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
					InnerVal: &ConfiguredActorsExternalSource{
						KeysPath: "./dev_config/keys/actors",
					},
				},
			}

			var sa SystemAuth
			err := yaml.Unmarshal([]byte(data), &sa)
			assert.NoError(err)
			assert.Equal(expected, sa)
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
  - externalId: bobdole
    namespace: root.admins
    key:
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
						&ConfiguredActor{
							ExternalId: "bobdole",
							Namespace:  "root.admins",
							Key: &Key{
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
			}

			var sa SystemAuth
			err := yaml.Unmarshal([]byte(data), &sa)
			assert.NoError(err)
			assert.Equal(expected, sa)
		})
	})
}
