package core

import (
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
)

// migrationHookPatch is the normalized object returned by connector-authored
// migration JavaScript. It records the configuration, label, annotation, and
// notification changes that should be applied after a hook runs successfully.
type migrationHookPatch struct {
	Config        migrationAnyPatch          `json:"config"`
	Labels        migrationStringPatch       `json:"labels"`
	Annotations   migrationStringPatch       `json:"annotations"`
	Notifications migrationNotificationPatch `json:"notifications"`
}

// migrationAnyPatch describes set/unset operations for JSON-like maps such as
// connection configuration, where values may be strings, numbers, objects, or
// arrays.
type migrationAnyPatch struct {
	Set   map[string]any `json:"set"`
	Unset []string       `json:"unset"`
}

// migrationStringPatch describes set/unset operations for string-valued maps
// such as connection labels and annotations.
type migrationStringPatch struct {
	Set   map[string]string `json:"set"`
	Unset []string          `json:"unset"`
}

// migrationNotificationPatch describes set/unset operations for
// connector-authored connection notices. Unset entries only use Key.
type migrationNotificationPatch struct {
	Set   []migrationNotificationDef `json:"set"`
	Unset []migrationNotificationDef `json:"unset"`
}

// migrationNotificationDef is the connector-authored notification payload that
// a hook can queue for users after the connection version changes.
type migrationNotificationDef struct {
	// Key is the connector-authored condition key used for dedupe and unsets.
	Key       string         `json:"key"`
	Level     string         `json:"level"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	ActionURL string         `json:"action_url"`
	Metadata  map[string]any `json:"metadata"`
}

// connectionMigrationCandidate is the in-memory target state assembled by hook
// execution and automatic migration analysis before the workflow writes the
// connection update and notification changes to persistent storage.
type connectionMigrationCandidate struct {
	Connection            *connection
	Target                *ConnectorVersion
	Config                map[string]any
	UserLabels            map[string]string
	Annotations           map[string]string
	SetupStep             *cschema.SetupStep
	SetupError            *string
	HealthState           database.ConnectionHealthState
	RefreshAuth           bool
	ProbeIdsToRun         []string
	Notifications         []database.NotificationUpsert
	NotificationKeys      []string
	NotificationUnsetKeys []string
	NotificationRank      int
}

func (c *connectionMigrationCandidate) NotificationKeysToResolve() []string {
	keys := append([]string{}, c.NotificationUnsetKeys...)
	for _, key := range []string{
		connectionNotificationKey(c, database.NotificationKeyAuthRequired),
		connectionNotificationKey(c, database.NotificationKeySetupRequired),
	} {
		if !containsString(c.NotificationKeys, key) {
			keys = appendUniqueString(keys, key)
		}
	}
	return keys
}
