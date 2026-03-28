package connectors

import (
	"encoding/json"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// SetupFlow defines the multi-step setup flow for a connector. Customers define this in their
// connector YAML to configure what forms are presented to users during connection setup.
type SetupFlow struct {
	// Preconnect defines form steps shown before the OAuth/auth flow begins.
	// Values collected here are available for mustache templating in auth configuration
	// (e.g. tenant subdomain in OAuth endpoints).
	Preconnect *SetupFlowPhase `json:"preconnect,omitempty" yaml:"preconnect,omitempty"`

	// Configure defines form steps shown after the auth flow completes.
	// These steps can use data sources that make proxied API calls using the
	// connection's credentials to populate dynamic form options.
	Configure *SetupFlowPhase `json:"configure,omitempty" yaml:"configure,omitempty"`
}

func (sf *SetupFlow) Validate(vc *common.ValidationContext) error {
	if sf == nil {
		return nil
	}

	result := &multierror.Error{}

	// Collect all step IDs across phases for uniqueness check
	seenIds := make(map[string]bool)

	if sf.Preconnect != nil {
		if err := sf.Preconnect.Validate(vc.PushField("preconnect"), seenIds, false); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if sf.Configure != nil {
		if err := sf.Configure.Validate(vc.PushField("configure"), seenIds, true); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}

// HasPreconnect returns true if the setup flow has preconnect steps.
func (sf *SetupFlow) HasPreconnect() bool {
	return sf != nil && sf.Preconnect != nil && len(sf.Preconnect.Steps) > 0
}

// HasConfigure returns true if the setup flow has configure steps.
func (sf *SetupFlow) HasConfigure() bool {
	return sf != nil && sf.Configure != nil && len(sf.Configure.Steps) > 0
}

// SetupFlowPhase is a sequential list of form steps within a phase (preconnect or configure).
type SetupFlowPhase struct {
	// Steps are the ordered form steps in this phase.
	Steps []SetupFlowStep `json:"steps" yaml:"steps"`
}

func (p *SetupFlowPhase) Validate(vc *common.ValidationContext, seenIds map[string]bool, allowDataSources bool) error {
	if p == nil {
		return nil
	}

	result := &multierror.Error{}

	if len(p.Steps) == 0 {
		result = multierror.Append(result, vc.NewError("must have at least one step"))
	}

	for i := range p.Steps {
		step := &p.Steps[i]
		if err := step.Validate(vc.PushField("steps").PushIndex(i), seenIds, allowDataSources); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}

// SetupFlowStep defines a single form step in the setup flow.
type SetupFlowStep struct {
	// Id is a unique identifier for this step within the connector's setup flow.
	Id string `json:"id" yaml:"id"`

	// Title is the human-readable title shown above the form.
	Title string `json:"title,omitempty" yaml:"title,omitempty"`

	// Description is additional explanatory text shown with the form.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// JsonSchema is the JSON Schema defining the form's data model and validation rules.
	JsonSchema common.RawJSON `json:"json_schema" yaml:"json_schema"`

	// UiSchema is the JSONForms UI Schema defining the form's layout and rendering.
	UiSchema common.RawJSON `json:"ui_schema,omitempty" yaml:"ui_schema,omitempty"`

	// DataSources defines dynamic data endpoints that can be referenced by form fields
	// using the x-data-source property in the JSON Schema. Only allowed in configure steps.
	DataSources map[string]DataSourceDef `json:"data_sources,omitempty" yaml:"data_sources,omitempty"`
}

func (s *SetupFlowStep) Validate(vc *common.ValidationContext, seenIds map[string]bool, allowDataSources bool) error {
	result := &multierror.Error{}

	if s.Id == "" {
		result = multierror.Append(result, vc.NewErrorfForField("id", "step id is required"))
	} else if seenIds[s.Id] {
		result = multierror.Append(result, vc.NewErrorfForField("id", "duplicate step id %q", s.Id))
	} else {
		seenIds[s.Id] = true
	}

	if s.JsonSchema.IsEmpty() {
		result = multierror.Append(result, vc.NewErrorfForField("json_schema", "json_schema is required"))
	} else if !json.Valid(s.JsonSchema) {
		result = multierror.Append(result, vc.NewErrorfForField("json_schema", "json_schema is not valid JSON"))
	}

	if !s.UiSchema.IsEmpty() && !json.Valid(s.UiSchema) {
		result = multierror.Append(result, vc.NewErrorfForField("ui_schema", "ui_schema is not valid JSON"))
	}

	if !allowDataSources && len(s.DataSources) > 0 {
		result = multierror.Append(result, vc.NewErrorfForField("data_sources", "data_sources are not allowed in preconnect steps (no credentials available yet)"))
	}

	for name, ds := range s.DataSources {
		if err := ds.Validate(vc.PushField("data_sources").PushField(name)); err != nil {
			result = multierror.Append(result, err)
		}
	}

	return result.ErrorOrNil()
}

// DataSourceDef defines how to fetch dynamic data for populating form fields.
type DataSourceDef struct {
	// ProxyRequest defines an HTTP request to make through the connection's authenticated proxy.
	ProxyRequest *DataSourceProxyRequest `json:"proxy_request,omitempty" yaml:"proxy_request,omitempty"`

	// Transform is a JavaScript expression that transforms the API response into
	// an array of {value, label} objects for use in dropdowns/selects.
	Transform string `json:"transform" yaml:"transform"`
}

func (d *DataSourceDef) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if d.ProxyRequest == nil {
		result = multierror.Append(result, vc.NewErrorfForField("proxy_request", "proxy_request is required"))
	} else {
		if err := d.ProxyRequest.Validate(vc.PushField("proxy_request")); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if d.Transform == "" {
		result = multierror.Append(result, vc.NewErrorfForField("transform", "transform expression is required"))
	}

	return result.ErrorOrNil()
}

// DataSourceProxyRequest defines the HTTP request to make for fetching data source options.
type DataSourceProxyRequest struct {
	// Method is the HTTP method (GET, POST, etc.).
	Method string `json:"method" yaml:"method"`

	// Url is the URL to request. Supports mustache templating with connection configuration
	// values, e.g. "https://{{cfg.tenant}}.example.com/api/v1/workspaces".
	Url string `json:"url" yaml:"url"`

	// Headers are additional HTTP headers to include in the request.
	// Values support mustache templating.
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

func (r *DataSourceProxyRequest) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}

	if r.Method == "" {
		result = multierror.Append(result, vc.NewErrorfForField("method", "method is required"))
	}

	if r.Url == "" {
		result = multierror.Append(result, vc.NewErrorfForField("url", "url is required"))
	}

	return result.ErrorOrNil()
}
