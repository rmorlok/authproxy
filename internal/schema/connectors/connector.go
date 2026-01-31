package connectors

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
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

	// Namespace is the namespace in which this connector lives. The value is a path to this namespace nested within
	// the parent namespaces. The path must begin with "root". If unspecified, the value is assumed to be "root".
	//
	// Example: `root/prod/some-feature`
	Namespace *string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

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
	Auth *Auth `json:"auth" yaml:"auth"`

	// Probes are a list of probes to run against connections of this connector type to validation the connection.
	Probes []Probe `json:"probes,omitempty" yaml:"probes,omitempty"`

	// Labels are the labels for the connector.
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
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
		clone.Auth = c.Auth.CloneValue()
	}

	if c.Labels != nil {
		clone.Labels = make(map[string]string, len(c.Labels))
		for k, v := range c.Labels {
			clone.Labels[k] = v
		}
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

	if c.Namespace != nil {
		if err := aschema.ValidateNamespacePath(*c.Namespace); err != nil {
			result = multierror.Append(result, err)
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

// HasNamespace returns true if the connector has a namespace set. This implies that the configuration set a namespace explicitly.
func (c *Connector) HasNamespace() bool {
	if c == nil {
		return false
	}

	return c.Namespace != nil
}

// IsDraft returns true if the connector has an explicitly defined state and that state is draft.
func (c *Connector) IsDraft() bool {
	if c == nil {
		return false
	}

	return c.State == "draft"
}

// GetNamespace returns the namespace of the connector. Defaults to root if unspecified.
func (c *Connector) GetNamespace() string {
	if c == nil || c.Namespace == nil {
		return aschema.RootNamespace
	}

	return *c.Namespace
}
