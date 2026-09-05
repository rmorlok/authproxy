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
	KeysPath         string               `json:"keysPath" yaml:"keysPath"`
	Permissions      []aschema.Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	SyncCronSchedule string               `json:"syncCronSchedule,omitempty" yaml:"syncCronSchedule,omitempty"`
}

func (s *ConfiguredActorsExternalSource) All() []*actorschema.Actor {
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
				Metadata: meta.ObjectMeta{Namespace: "root"},
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

func (s *ConfiguredActorsExternalSource) GetByExternalId(externalId string) (*actorschema.Actor, bool) {
	for _, actor := range s.All() {
		if actor.Spec.ExternalId == externalId {
			return actor, true
		}
	}

	return nil, false
}

func (s *ConfiguredActorsExternalSource) GetBySubject(subject string) (*actorschema.Actor, bool) {
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
