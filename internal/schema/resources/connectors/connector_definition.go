package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

// ConnectorDefinition contains only the provider behavior for a connector
// generation. Logical identity, generation, release intent, and observed state
// belong to Connector metadata, spec.release, and status respectively.
//
// This is the only value serialized into the encrypted connector definition
// column. Keep resource metadata out of this type so the storage boundary is
// enforceable by construction.
type ConnectorDefinition struct {
	// DisplayName is the human readable name of the connector. This is
	// displayed to the user in the marketplace portal.
	DisplayName string `json:"displayName" yaml:"displayName"`

	// Logo is the logo of the connector. This is displayed to the user in the
	// marketplace portal.
	Logo *common.Image `json:"logo" yaml:"logo"`

	// Highlight is a short blurb about the connector. This is displayed to the
	// user in the marketplace portal.
	Highlight string `json:"highlight,omitempty" yaml:"highlight,omitempty"`

	// Description is a longer description of the connector. This is displayed
	// to the user in the marketplace portal.
	Description string `json:"description" yaml:"description"`

	// StatusPageUrl is a URL to the status page for the external service this
	// connector integrates with. This helps users track 3rd party outages that
	// may affect their connections.
	StatusPageUrl string `json:"statusPageUrl,omitempty" yaml:"statusPageUrl,omitempty"`

	// MarketplaceUrl is a URL to the marketplace listing for this connector's
	// external service. For example, this could link to the app's listing in
	// the service's app marketplace.
	MarketplaceUrl string `json:"marketplaceUrl,omitempty" yaml:"marketplaceUrl,omitempty"`

	// DeveloperConsoleUrl is a URL to the developer console for this
	// connector's external service. This is where developers manage their app's
	// configuration, API keys, etc.
	DeveloperConsoleUrl string `json:"developerConsoleUrl,omitempty" yaml:"developerConsoleUrl,omitempty"`

	// OAuthClientUrl is a URL to the OAuth client management page for this
	// connector's external service. This is typically a sub-page of the
	// developer console where the OAuth client credentials are managed.
	OAuthClientUrl string `json:"oauthClientUrl,omitempty" yaml:"oauthClientUrl,omitempty"`

	// Auth is how this connector authenticates. Possible values are of type
	// OAuth2 or APIKey. See individual documentation for each struct for more
	// details.
	Auth *Auth `json:"auth" yaml:"auth"`

	// Javascript is connector-level JavaScript that defines shared constants
	// and functions available to connector-authored predicates and transforms.
	Javascript string `json:"javascript,omitempty" yaml:"javascript,omitempty"`

	// Migrations are optional connector-authored hooks used to migrate an
	// existing connection's stored configuration, labels, and annotations
	// between connector versions.
	Migrations *Migrations `json:"migrations,omitempty" yaml:"migrations,omitempty"`

	// RateLimiting configures how 429 rate limiting responses from the 3rd
	// party are handled. If unset, default behavior is enabled (parse
	// Retry-After header, 60s default backoff).
	RateLimiting *RateLimiting `json:"rateLimiting,omitempty" yaml:"rateLimiting,omitempty"`

	// Probes are a list of probes to run against connections of this connector
	// type to validation the connection.
	Probes []Probe `json:"probes,omitempty" yaml:"probes,omitempty"`

	// SetupFlow defines the multi-step setup flow for configuring connections.
	// Includes optional preconnect forms (before auth) and configure forms
	// (after auth).
	SetupFlow *SetupFlow `json:"setupFlow,omitempty" yaml:"setupFlow,omitempty"`

	// Telemetry carries per-connector overrides for OpenTelemetry behavior
	// on outbound calls routed through this connector. See ConnectorTelemetry.
	Telemetry *ConnectorTelemetry `json:"telemetry,omitempty" yaml:"telemetry,omitempty"`
}

func (c *ConnectorDefinition) Clone() *ConnectorDefinition {
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

	if c.RateLimiting != nil {
		clone.RateLimiting = c.RateLimiting.Clone()
	}

	if c.Migrations != nil {
		clone.Migrations = c.Migrations.Clone()
	}

	clone.Telemetry = c.Telemetry.Clone()

	return &clone
}

func (c *ConnectorDefinition) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}
	javascript, err := apjs.CompileAndValidateLibrary(c.Javascript)
	if err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "invalid connector javascript: %v", err))
		javascript = nil
	}

	if c.RateLimiting != nil {
		if err := c.RateLimiting.Validate(vc.PushField("rate_limiting")); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if c.Migrations != nil {
		if err := c.Migrations.Validate(vc.PushField("migrations")); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if c.Auth != nil {
		if av, ok := c.Auth.Inner().(AuthJavascriptValidator); ok {
			if err := av.ValidateWithJavascript(vc.PushField("auth"), javascript); err != nil {
				result = multierror.Append(result, err)
			}
		} else if av, ok := c.Auth.Inner().(AuthValidator); ok {
			if err := av.Validate(vc.PushField("auth")); err != nil {
				result = multierror.Append(result, err)
			}
		}
	}

	for i, probe := range c.Probes {
		if err := probe.ValidateWithJavascript(vc.PushField("probes").PushIndex(i), javascript); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if c.SetupFlow != nil {
		if err := c.SetupFlow.ValidateWithJavascript(vc.PushField("setup_flow"), javascript); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if err := c.validateMustacheReferences(vc); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

// Hash computes a semantic hash of the connector data. It does not account for
// data that is not stored in the configuration directly (e.g. environment
// variables referenced). A change in the hash implies that a new version must
// be created if the existing version is already live.
func (c *ConnectorDefinition) Hash() string {
	if c == nil {
		return ""
	}

	hash, err := meta.SemanticSpecHash(c)
	if err != nil {
		return ""
	}

	return hash
}

// HasProbes returns true if the connector has one or more probes, false
// otherwise.
func (c *ConnectorDefinition) HasProbes() bool {
	if c == nil {
		return false
	}

	return len(c.Probes) > 0
}
