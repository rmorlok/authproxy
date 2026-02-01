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
  cookie_domain: localhost:8080
  jwt_signing_key:
    public_key:
      path: ./dev_config/keys/system.pub
    private_key:
      path: ./dev_config/keys/system
  actors:
    keys_path: ./dev_config/keys/actors
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
cookie_domain: localhost:8080
jwt_signing_key:
  public_key:
    path: ./dev_config/keys/system.pub
  private_key:
    path: ./dev_config/keys/system
actors:
  - external_id: bobdole
    key:
      public_key:
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
