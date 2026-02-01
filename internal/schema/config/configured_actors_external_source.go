package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type ConfiguredActorsExternalSource struct {
	KeysPath         string               `json:"keys_path" yaml:"keys_path"`
	Permissions      []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	SyncCronSchedule string               `json:"sync_cron_schedule,omitempty" yaml:"sync_cron_schedule,omitempty"`
}

func (s *ConfiguredActorsExternalSource) All() []*ConfiguredActor {
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

func (s *ConfiguredActorsExternalSource) GetByExternalId(externalId string) (*ConfiguredActor, bool) {
	for _, actor := range s.All() {
		if actor.ExternalId == externalId {
			return actor, true
		}
	}

	return nil, false
}

func (s *ConfiguredActorsExternalSource) GetBySubject(subject string) (*ConfiguredActor, bool) {
	// Subject is the same as ExternalId (no admin/ prefix handling)
	return s.GetByExternalId(subject)
}

// GetSyncCronScheduleOrDefault returns the cron schedule for actors sync,
// or a default of every 5 minutes if not configured.
func (s *ConfiguredActorsExternalSource) GetSyncCronScheduleOrDefault() string {
	if s == nil || s.SyncCronSchedule == "" {
		return "*/5 * * * *" // Every 5 minutes
	}
	return s.SyncCronSchedule
}

var _ ConfiguredActorsType = (*ConfiguredActorsExternalSource)(nil)
