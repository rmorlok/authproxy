package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type AdminUsersExternalSource struct {
	KeysPath         string               `json:"keys_path" yaml:"keys_path"`
	Permissions      []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	SyncCronSchedule string               `json:"sync_cron_schedule,omitempty" yaml:"sync_cron_schedule,omitempty"`
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

func (s *AdminUsersExternalSource) GetByJwtSubject(subject string) (*AdminUser, bool) {
	if !strings.HasPrefix(subject, "admin/") {
		return nil, false
	}

	username := strings.TrimPrefix(subject, "admin/")
	return s.GetByUsername(username)
}

// GetSyncCronScheduleOrDefault returns the cron schedule for admin users sync,
// or a default of every 5 minutes if not configured.
func (sa *AdminUsersExternalSource) GetSyncCronScheduleOrDefault() string {
	if sa == nil || sa.SyncCronSchedule == "" {
		return "*/5 * * * *" // Every 5 minutes
	}
	return sa.SyncCronSchedule
}

var _ AdminUsersType = (*AdminUsersExternalSource)(nil)
