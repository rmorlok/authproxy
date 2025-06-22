package connectors

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/config/common"
)

type AuthOAuth2 struct {
	Type          AuthType                `json:"type" yaml:"type"`
	ClientId      common.StringValue      `json:"client_id" yaml:"client_id"`
	ClientSecret  common.StringValue      `json:"client_secret" yaml:"client_secret"`
	Scopes        []Scope                 `json:"scopes" yaml:"scopes"`
	Authorization AuthOauth2Authorization `json:"authorization" yaml:"authorization"`
	Token         AuthOauth2Token         `json:"token" yaml:"token"`
}

func (i *AuthOAuth2) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("auth oauth2 expected a mapping node, got %s", common.KindToString(value.Kind))
	}

	var clientIdSecret common.StringValue
	var clientSecretSecret common.StringValue

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "client_id":
			if clientIdSecret, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		case "client_secret":
			if clientSecretSecret, err = common.StringValueUnmarshalYAML(valueNode); err != nil {
				return err
			}
			matched = true
		}

		if matched {
			// Remove the key/value from the raw unmarshalling, and pull back our index
			// because of the changing slice size to the left of what we are indexing
			value.Content = append(value.Content[:i], value.Content[i+2:]...)
			i -= 2
		}
	}

	// Let the rest unmarshall normally
	type RawType AuthOAuth2
	raw := (*RawType)(i)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.ClientId = clientIdSecret
	raw.ClientSecret = clientSecretSecret
	raw.Type = AuthTypeOAuth2

	return nil
}

func (a *AuthOAuth2) GetType() AuthType {
	return AuthTypeOAuth2
}

// UnmarshalJSON implements custom JSON unmarshalling for the AuthOAuth2 struct
func (a *AuthOAuth2) UnmarshalJSON(data []byte) error {
	// Define a temporary struct with the same fields as AuthOAuth2
	// but with ClientId and ClientSecret as json.RawMessage to capture their raw JSON
	type TempAuthOAuth2 struct {
		Type          AuthType                `json:"type"`
		ClientId      json.RawMessage         `json:"client_id"`
		ClientSecret  json.RawMessage         `json:"client_secret"`
		Scopes        []Scope                 `json:"scopes"`
		Authorization AuthOauth2Authorization `json:"authorization"`
		Token         AuthOauth2Token         `json:"token"`
	}

	var temp TempAuthOAuth2

	// Unmarshal into the temporary struct
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Copy the simple fields
	a.Type = temp.Type
	a.Scopes = temp.Scopes
	a.Authorization = temp.Authorization
	a.Token = temp.Token

	// Handle ClientId if it's not null
	if len(temp.ClientId) > 0 && string(temp.ClientId) != "null" {
		// Try to determine the type of StringValue from the JSON
		var clientIdMap map[string]interface{}
		if err := json.Unmarshal(temp.ClientId, &clientIdMap); err != nil {
			return err
		}

		var clientId common.StringValue
		if _, ok := clientIdMap["value"]; ok {
			// It's a StringValueDirect
			var svDirect common.StringValueDirect
			if err := json.Unmarshal(temp.ClientId, &svDirect); err != nil {
				return err
			}
			clientId = &svDirect
		} else if _, ok := clientIdMap["base64"]; ok {
			// It's a StringValueBase64
			var svBase64 common.StringValueBase64
			if err := json.Unmarshal(temp.ClientId, &svBase64); err != nil {
				return err
			}
			clientId = &svBase64
		} else if _, ok := clientIdMap["env_var"]; ok {
			// It's a StringValueEnvVar
			var svEnvVar common.StringValueEnvVar
			if err := json.Unmarshal(temp.ClientId, &svEnvVar); err != nil {
				return err
			}
			clientId = &svEnvVar
		} else if _, ok := clientIdMap["env_var_base64"]; ok {
			// It's a StringValueEnvVarBase64
			var svEnvVarBase64 common.StringValueEnvVarBase64
			if err := json.Unmarshal(temp.ClientId, &svEnvVarBase64); err != nil {
				return err
			}
			clientId = &svEnvVarBase64
		} else if _, ok := clientIdMap["path"]; ok {
			// It's a StringValueFile
			var svFile common.StringValueFile
			if err := json.Unmarshal(temp.ClientId, &svFile); err != nil {
				return err
			}
			clientId = &svFile
		}

		a.ClientId = clientId
	}

	// Handle ClientSecret if it's not null
	if len(temp.ClientSecret) > 0 && string(temp.ClientSecret) != "null" {
		// Try to determine the type of StringValue from the JSON
		var clientSecretMap map[string]interface{}
		if err := json.Unmarshal(temp.ClientSecret, &clientSecretMap); err != nil {
			return err
		}

		var clientSecret common.StringValue
		if _, ok := clientSecretMap["value"]; ok {
			// It's a StringValueDirect
			var svDirect common.StringValueDirect
			if err := json.Unmarshal(temp.ClientSecret, &svDirect); err != nil {
				return err
			}
			clientSecret = &svDirect
		} else if _, ok := clientSecretMap["base64"]; ok {
			// It's a StringValueBase64
			var svBase64 common.StringValueBase64
			if err := json.Unmarshal(temp.ClientSecret, &svBase64); err != nil {
				return err
			}
			clientSecret = &svBase64
		} else if _, ok := clientSecretMap["env_var"]; ok {
			// It's a StringValueEnvVar
			var svEnvVar common.StringValueEnvVar
			if err := json.Unmarshal(temp.ClientSecret, &svEnvVar); err != nil {
				return err
			}
			clientSecret = &svEnvVar
		} else if _, ok := clientSecretMap["env_var_base64"]; ok {
			// It's a StringValueEnvVarBase64
			var svEnvVarBase64 common.StringValueEnvVarBase64
			if err := json.Unmarshal(temp.ClientSecret, &svEnvVarBase64); err != nil {
				return err
			}
			clientSecret = &svEnvVarBase64
		} else if _, ok := clientSecretMap["path"]; ok {
			// It's a StringValueFile
			var svFile common.StringValueFile
			if err := json.Unmarshal(temp.ClientSecret, &svFile); err != nil {
				return err
			}
			clientSecret = &svFile
		}

		a.ClientSecret = clientSecret
	}

	return nil
}

func (a *AuthOAuth2) Clone() Auth {
	if a == nil {
		return nil
	}

	clone := *a

	clone.ClientId = a.ClientId.Clone()
	clone.ClientSecret = a.ClientSecret.Clone()

	scopes := make([]Scope, 0, len(a.Scopes))
	for _, scope := range a.Scopes {
		scopes = append(scopes, scope)
	}
	clone.Scopes = scopes

	return &clone
}
