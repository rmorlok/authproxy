package actor

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

func (a ActorSpecPatch) MarshalJSON() ([]byte, error) {
	value := make(map[string]any, 3)
	if a.HasExternalId() {
		value["externalId"] = a.ExternalId
	}
	if a.HasPermissions() {
		value["permissions"] = a.Permissions
	}
	if a.HasSigningKey() {
		value["signingKey"] = a.SigningKey
	}
	return json.Marshal(value)
}

func (a *ActorSpecPatch) UnmarshalJSON(data []byte) error {
	var wire struct {
		ExternalId  json.RawMessage `json:"externalId"`
		Permissions json.RawMessage `json:"permissions"`
		SigningKey  json.RawMessage `json:"signingKey"`
	}
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}

	*a = ActorSpecPatch{}
	if wire.ExternalId != nil {
		a.externalIdPresent = true
		if err := util.DecodeJSONStrict(wire.ExternalId, &a.ExternalId); err != nil {
			return err
		}
	}
	if wire.Permissions != nil {
		a.permissionsPresent = true
		if err := util.DecodeJSONStrict(wire.Permissions, &a.Permissions); err != nil {
			return err
		}
	}
	if wire.SigningKey != nil {
		a.signingKeyPresent = true
		if err := util.DecodeJSONStrict(wire.SigningKey, &a.SigningKey); err != nil {
			return err
		}
	}
	return nil
}

func (a ActorSpecPatch) MarshalYAML() (any, error) {
	value := make(map[string]any, 3)
	if a.HasExternalId() {
		value["externalId"] = a.ExternalId
	}
	if a.HasPermissions() {
		value["permissions"] = a.Permissions
	}
	if a.HasSigningKey() {
		value["signingKey"] = a.SigningKey
	}
	return value, nil
}

func (a *ActorSpecPatch) UnmarshalYAML(value *yaml.Node) error {
	type plain ActorSpecPatch
	var decoded plain
	if err := util.DecodeYAMLNodeStrict(value, &decoded); err != nil {
		return err
	}

	node := value
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	present := map[string]bool{}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			present[node.Content[i].Value] = true
		}
	}

	*a = ActorSpecPatch(decoded)
	a.externalIdPresent = present["externalId"]
	a.permissionsPresent = present["permissions"]
	a.signingKeyPresent = present["signingKey"]
	return nil
}
