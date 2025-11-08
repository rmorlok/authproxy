package connectors

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/internal/config/common"
)

type Connector struct {
	// Id is the global id for this connector. This does not change version to version. In the config file, this can
	// be omitted if there is only one instance of a particular type of connector. If there are multiple connectors that
	// share the same type, the ids would need to be explicitly defined.
	Id uuid.UUID `json:"id" yaml:"id"`

	// Type is the logical type of the connector. Multiple connectors can implement the same type. This might represent
	// two different ways of connecting to the same 3rd party system (e.g. via API key and OAuth) where the types of auth
	// don't have an impact on the capabilities of the connector.
	Type string `json:"type" yaml:"type"`

	// Version is the logical version of the connector. When auth materially changes, such as adding new scopes,
	// changing client ids/secrets, adding configuration settings, etc. the logical version of the connector must change
	// so that existing connections can be managed through the migration process. If specified explicitly in the config
	// file, this version will prevent changes to the system by preventing startup. If unspecified, the system will
	// automatically create versions based on the configuration changing. If specified explicitly, this version must
	// start with 1 (zero implies unspecified).
	Version uint64 `json:"version,omitempty" yaml:"version,omitempty"`

	// State is the release state of the connector. Must either be primary or draft if specified. Defaults to primary
	// if unspecified.
	State string `json:"state,omitempty" yaml:"state,omitempty"`

	// DisplayName is the human readable name of the connector. This is displayed to the user in the marketplace portal.
	DisplayName string `json:"display_name" yaml:"display_name"`

	// Logo is the logo of the connector. This is displayed to the user in the marketplace portal.
	Logo *common.Image `json:"logo" yaml:"logo"`

	// Highlight is a short blurb about the connector. This is displayed to the user in the marketplace portal.
	Highlight string `json:"highlight,omitempty" yaml:"highlight,omitempty"`

	// Description is a longer description of the connector. This is displayed to the user in the marketplace portal.
	Description string `json:"description" yaml:"description"`

	// Auth is how this connector authenticates. Possible values are of type OAuth2 or APIKey. See individual
	// documentation for each struct for more details.
	Auth Auth `json:"auth" yaml:"auth"`

	// Probes are a list of probes to run against connections of this connector type to validation the connection.
	Probes []Probe `json:"probes,omitempty" yaml:"probes,omitempty"`
}

func (c *Connector) UnmarshalYAML(value *yaml.Node) error {
	// Ensure the node is a mapping node
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("connector expected a mapping node, got %s", common.KindToString(value.Kind))
	}

	var auth Auth

	// Handle custom unmarshalling for some attributes. Iterate through the mapping node's content,
	// which will be sequences of keys, then values.
	for i := 0; i < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valueNode := value.Content[i+1]

		var err error
		matched := false

		switch keyNode.Value {
		case "auth":
			if auth, err = authUnmarshalYAML(valueNode); err != nil {
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
	type RawType Connector
	raw := (*RawType)(c)
	if err := value.Decode(raw); err != nil {
		return err
	}

	// Set the custom unmarshalled types
	raw.Auth = auth

	return nil
}

// UnmarshalJSON implements custom JSON unmarshalling for the Connector struct
func (c *Connector) UnmarshalJSON(data []byte) error {
	// Define a temporary struct with the same fields as Connector
	// but with Logo and Auth as json.RawMessage to capture their raw JSON
	type TempConnector struct {
		Id          uuid.UUID       `json:"id"`
		Type        string          `json:"type"`
		Version     uint64          `json:"version,omitempty"`
		State       string          `json:"state,omitempty"`
		DisplayName string          `json:"display_name"`
		Logo        *common.Image   `json:"logo"`
		Highlight   string          `json:"highlight"`
		Description string          `json:"description"`
		Auth        json.RawMessage `json:"auth"`
	}

	var temp TempConnector

	// Unmarshal into the temporary struct
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Copy the simple fields
	c.Id = temp.Id
	c.Type = temp.Type
	c.Version = temp.Version
	c.State = temp.State
	c.Highlight = temp.Highlight
	c.DisplayName = temp.DisplayName
	c.Logo = temp.Logo
	c.Description = temp.Description

	// Handle Auth if it's not null
	if len(temp.Auth) > 0 && string(temp.Auth) != "null" {
		// Try to determine the type of auth from the JSON
		var authMap map[string]interface{}
		if err := json.Unmarshal(temp.Auth, &authMap); err != nil {
			return err
		}

		var auth Auth
		if typeVal, ok := authMap["type"]; ok {
			switch AuthType(typeVal.(string)) {
			case AuthTypeOAuth2:
				// It's an AuthOAuth2
				var authOAuth2 AuthOAuth2
				if err := json.Unmarshal(temp.Auth, &authOAuth2); err != nil {
					return err
				}
				auth = &authOAuth2
			case AuthTypeAPIKey:
				// It's an AuthApiKey
				var authApiKey AuthApiKey
				if err := json.Unmarshal(temp.Auth, &authApiKey); err != nil {
					return err
				}
				auth = &authApiKey
			}
		}

		c.Auth = auth
	}

	return nil
}

func (c *Connector) Clone() *Connector {
	if c == nil {
		return nil
	}

	clone := *c

	if c.Logo != nil {
		clone.Logo = c.Logo.CloneImage()
	}

	if c.Auth != nil {
		clone.Auth = c.Auth.Clone()
	}

	return &clone
}

func (c *Connector) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if c.Type == "" {
		result = multierror.Append(result, vc.NewErrorfForField("type", "connector must have type"))
	}

	if c.State != "" {
		if c.State != "draft" && c.State != "primary" {
			result = multierror.Append(result, vc.NewErrorfForField("state", "connector state must be either draft or primary"))
		}
	}

	for i, probe := range c.Probes {
		if err := probe.Validate(vc.PushField("probes").PushIndex(i)); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}

// Hash computes a semantic hash of the connector data. It does not account for data that is not stored in the
// configuration directly (e.g. environment variables referenced). A change in the hash implies that a new version
// must be created if the existing version is already live.
func (c *Connector) Hash() string {
	jsonData, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	h := sha1.New()
	h.Write(jsonData)
	return hex.EncodeToString(h.Sum(nil))[:7]
}

// HasUuid returns true if the connector has a UUID. This implies that the configuration set a UUID explicitly.
func (c *Connector) HasId() bool {
	if c == nil {
		return false
	}

	return c.Id != uuid.Nil
}

// HasVersion returns true if the connector has a version. This implies that the configuration set a version explicitly.
func (c *Connector) HasVersion() bool {
	if c == nil {
		return false
	}

	return c.Version > 0
}

// HasState returns true if the connector has a state. This implies that the configuration set a state explicitly.
func (c *Connector) HasState() bool {
	if c == nil {
		return false
	}

	return c.State != ""
}

// IsDraft returns true if the connector has an explicitly defined state and that state is draft.
func (c *Connector) IsDraft() bool {
	if c == nil {
		return false
	}

	return c.State == "draft"
}
