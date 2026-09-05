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
      namespace: root
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
							Metadata: meta.ObjectMeta{Namespace: "root"},
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
