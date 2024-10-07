package config

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

type AdminUsersExternalSource struct {
	KeysPath string `json:"keys_path" yaml:"keys_path"`
}

func (s *AdminUsersExternalSource) All() []*AdminUser {
	entries, err := os.ReadDir(s.KeysPath)
	if err != nil {
		panic(err)
	}

	adminUsers := make([]*AdminUser, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			// Skip directories
			continue
		}

		// Check if the file has the desired extension
		if strings.HasSuffix(entry.Name(), ".pub") {
			username := strings.TrimSuffix(entry.Name(), ".pub")
			adminUsers = append(adminUsers, &AdminUser{
				Username: username,
				Key: &KeyPublicPrivate{
					PublicKey: &KeyDataFile{
						Path: filepath.Join(s.KeysPath, entry.Name()),
					},
				},
			})
		}
	}

	return adminUsers
}

func (s *AdminUsersExternalSource) GetByUsername(username string) (*AdminUser, bool) {
	for _, adminUser := range s.All() {
		if adminUser.Username == username {
			return adminUser, true
		}
	}

	return nil, false
}

func UnmarshallYamlAdminUsersExternalSourceString(data string) (*AdminUsersExternalSource, error) {
	return UnmarshallYamlAdminUsersExternalSource([]byte(data))
}

func UnmarshallYamlAdminUsersExternalSource(data []byte) (*AdminUsersExternalSource, error) {
	var aues AdminUsersExternalSource
	if err := yaml.Unmarshal(data, &aues); err != nil {
		return nil, err
	}

	return &aues, nil
}
