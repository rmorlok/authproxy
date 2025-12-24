package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemAuth(t *testing.T) {
	assert := require.New(t)

	t.Run("yaml parse", func(t *testing.T) {
		t.Run("admin users path", func(t *testing.T) {
			data := `
  cookie_domain: localhost:8080
  jwt_signing_key:
    public_key:
      path: ./dev_config/keys/system.pub
    private_key:
      path: ./dev_config/keys/system
  admin_users:
    keys_path: ./dev_config/keys/admin
`
			expected := &SystemAuth{
				JwtSigningKey: &KeyPublicPrivate{
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
				AdminUsers: &AdminUsersExternalSource{
					KeysPath: "./dev_config/keys/admin",
				},
			}

			sa, err := UnmarshallYamlSystemAuthString(data)
			assert.NoError(err)
			assert.Equal(expected, sa)
		})
		t.Run("admin users list", func(t *testing.T) {
			data := `
cookie_domain: localhost:8080
jwt_signing_key:
  public_key:
    path: ./dev_config/keys/system.pub
  private_key:
    path: ./dev_config/keys/system
admin_users:
  - username: bobdole
    key:
      public_key:
        path: ./dev_config/keys/admin/bobdole.pub
`
			expected := &SystemAuth{
				JwtSigningKey: &KeyPublicPrivate{
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
				AdminUsers: AdminUsersList{
					&AdminUser{
						Username: "bobdole",
						Key: &KeyPublicPrivate{
							PublicKey: &KeyData{
								InnerVal: &KeyDataFile{
									Path: "./dev_config/keys/admin/bobdole.pub",
								},
							},
						},
					},
				},
			}

			sa, err := UnmarshallYamlSystemAuthString(data)
			assert.NoError(err)
			assert.Equal(expected, sa)
		})
	})
}
