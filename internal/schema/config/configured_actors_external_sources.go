package config

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

const configuredActorsSyncCronScheduleKey = "syncCronSchedule"

// ConfiguredActorsExternalSources groups public-key directories by the
// namespace that owns the actors loaded from each directory.
type ConfiguredActorsExternalSources struct {
	Sources          map[string]*ConfiguredActorsExternalSource
	SyncCronSchedule string
}

func (s *ConfiguredActorsExternalSources) All() []*ConfiguredActor {
	if s == nil {
		return nil
	}

	namespaces := make([]string, 0, len(s.Sources))
	for namespace := range s.Sources {
		namespaces = append(namespaces, namespace)
	}
	slices.Sort(namespaces)

	var actors []*ConfiguredActor
	for _, namespace := range namespaces {
		actors = append(actors, s.Sources[namespace].AllInNamespace(namespace)...)
	}
	return actors
}

func (s *ConfiguredActorsExternalSources) GetByExternalId(externalId string) (*ConfiguredActor, bool) {
	for _, actor := range s.All() {
		if actor.ExternalId == externalId {
			return actor, true
		}
	}
	return nil, false
}

func (s *ConfiguredActorsExternalSources) GetBySubject(subject string) (*ConfiguredActor, bool) {
	return s.GetByExternalId(subject)
}

func (s *ConfiguredActorsExternalSources) GetSyncCronScheduleOrDefault() string {
	if s == nil || s.SyncCronSchedule == "" {
		return "*/5 * * * *"
	}
	return s.SyncCronSchedule
}

func (s *ConfiguredActorsExternalSources) MarshalJSON() ([]byte, error) {
	values := make(map[string]any, len(s.Sources)+1)
	for namespace, source := range s.Sources {
		values[namespace] = source
	}
	if s.SyncCronSchedule != "" {
		values[configuredActorsSyncCronScheduleKey] = s.SyncCronSchedule
	}
	return json.Marshal(values)
}

func (s *ConfiguredActorsExternalSources) UnmarshalJSON(data []byte) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}

	s.Sources = make(map[string]*ConfiguredActorsExternalSource, len(values))
	for namespace, raw := range values {
		if namespace == configuredActorsSyncCronScheduleKey {
			if err := json.Unmarshal(raw, &s.SyncCronSchedule); err != nil {
				return fmt.Errorf("invalid %s: %w", configuredActorsSyncCronScheduleKey, err)
			}
			continue
		}
		if err := ValidateNamespacePath(namespace); err != nil {
			return fmt.Errorf("invalid actor source namespace %q: %w", namespace, err)
		}
		var source ConfiguredActorsExternalSource
		if err := util.DecodeJSONStrict(raw, &source); err != nil {
			return fmt.Errorf("invalid actor source %q: %w", namespace, err)
		}
		s.Sources[namespace] = &source
	}
	if len(s.Sources) == 0 {
		return fmt.Errorf("at least one namespaced actor source is required")
	}
	return nil
}

func (s *ConfiguredActorsExternalSources) MarshalYAML() (any, error) {
	values := make(map[string]any, len(s.Sources)+1)
	for namespace, source := range s.Sources {
		values[namespace] = source
	}
	if s.SyncCronSchedule != "" {
		values[configuredActorsSyncCronScheduleKey] = s.SyncCronSchedule
	}
	return values, nil
}

func (s *ConfiguredActorsExternalSources) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("namespaced actor sources expected a mapping node, got %s", KindToString(value.Kind))
	}

	s.Sources = make(map[string]*ConfiguredActorsExternalSource, len(value.Content)/2)
	for i := 0; i < len(value.Content); i += 2 {
		namespace := value.Content[i].Value
		sourceNode := value.Content[i+1]
		if namespace == configuredActorsSyncCronScheduleKey {
			if err := sourceNode.Decode(&s.SyncCronSchedule); err != nil {
				return fmt.Errorf("invalid %s: %w", configuredActorsSyncCronScheduleKey, err)
			}
			continue
		}
		if err := ValidateNamespacePath(namespace); err != nil {
			return fmt.Errorf("invalid actor source namespace %q: %w", namespace, err)
		}
		var source ConfiguredActorsExternalSource
		if err := util.DecodeYAMLNodeStrict(sourceNode, &source); err != nil {
			return fmt.Errorf("invalid actor source %q: %w", namespace, err)
		}
		s.Sources[namespace] = &source
	}
	if len(s.Sources) == 0 {
		return fmt.Errorf("at least one namespaced actor source is required")
	}
	return nil
}

var _ ConfiguredActorsType = (*ConfiguredActorsExternalSources)(nil)
