package connectors

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/go-multierror"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"

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

// TotalSteps returns the total number of form steps across both phases.
func (sf *SetupFlow) TotalSteps() int {
	if sf == nil {
		return 0
	}
	total := 0
	if sf.Preconnect != nil {
		total += len(sf.Preconnect.Steps)
	}
	if sf.Configure != nil {
		total += len(sf.Configure.Steps)
	}
	return total
}

// GetStepBySetupStep returns the step definition and its 0-based global index for the given
// setup step string (e.g. "preconnect:0", "configure:1").
func (sf *SetupFlow) GetStepBySetupStep(setupStep string) (*SetupFlowStep, int, error) {
	phase, index, err := ParseSetupStep(setupStep)
	if err != nil {
		return nil, 0, err
	}

	switch phase {
	case "preconnect":
		if sf.Preconnect == nil || index >= len(sf.Preconnect.Steps) {
			return nil, 0, fmt.Errorf("preconnect step index %d out of range", index)
		}
		return &sf.Preconnect.Steps[index], index, nil
	case "configure":
		if sf.Configure == nil || index >= len(sf.Configure.Steps) {
			return nil, 0, fmt.Errorf("configure step index %d out of range", index)
		}
		globalIndex := index
		if sf.Preconnect != nil {
			globalIndex += len(sf.Preconnect.Steps)
		}
		return &sf.Configure.Steps[index], globalIndex, nil
	default:
		return nil, 0, fmt.Errorf("unknown phase %q", phase)
	}
}

// FirstSetupStep returns the setup step string for the first step in the flow.
// Returns "preconnect:0" if preconnect steps exist, otherwise "configure:0".
// Returns empty string if no steps exist.
func (sf *SetupFlow) FirstSetupStep() string {
	if sf == nil {
		return ""
	}
	if sf.HasPreconnect() {
		return "preconnect:0"
	}
	if sf.HasConfigure() {
		return "configure:0"
	}
	return ""
}

// SetupStepVerify is the pseudo-step that indicates connection probes are running in the background
// to verify credentials obtained during auth.
const SetupStepVerify = "verify"

// SetupStepVerifyFailed is a terminal pseudo-step that indicates probe verification failed.
// The connection's setup_error column holds the failure message. It is not part of the normal
// linear flow; the UI surfaces an error screen with retry/cancel options.
const SetupStepVerifyFailed = "verify_failed"

// NextSetupStep returns the next setup step after the given one, or empty string if done.
// The auth phase is implicit between preconnect and configure phases. When the connector has
// probes, a verify phase runs between auth and configure.
func (sf *SetupFlow) NextSetupStep(current string, hasProbes bool) (string, error) {
	phase, index, err := ParseSetupStep(current)
	if err != nil {
		return "", err
	}

	switch phase {
	case "preconnect":
		if sf.Preconnect != nil && index+1 < len(sf.Preconnect.Steps) {
			return fmt.Sprintf("preconnect:%d", index+1), nil
		}
		// Preconnect done — next is auth
		return "auth", nil
	case "auth":
		if hasProbes {
			return SetupStepVerify, nil
		}
		if sf.HasConfigure() {
			return "configure:0", nil
		}
		return "", nil // Complete
	case SetupStepVerify:
		if sf.HasConfigure() {
			return "configure:0", nil
		}
		return "", nil // Complete
	case "configure":
		if sf.Configure != nil && index+1 < len(sf.Configure.Steps) {
			return fmt.Sprintf("configure:%d", index+1), nil
		}
		return "", nil // Complete
	default:
		return "", fmt.Errorf("unknown phase %q", phase)
	}
}

// ParseSetupStep parses a setup step string like "preconnect:0" into phase and index.
// Singleton pseudo-steps "auth", "verify", and "verify_failed" return (phase, 0, nil).
func ParseSetupStep(setupStep string) (phase string, index int, err error) {
	switch setupStep {
	case "auth", SetupStepVerify, SetupStepVerifyFailed:
		return setupStep, 0, nil
	}

	parts := strings.SplitN(setupStep, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("invalid setup step format %q", setupStep)
	}

	phase = parts[0]
	if phase != "preconnect" && phase != "configure" {
		return "", 0, fmt.Errorf("invalid setup step phase %q", phase)
	}

	index, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid setup step index %q: %w", parts[1], err)
	}

	if index < 0 {
		return "", 0, fmt.Errorf("setup step index must be non-negative, got %d", index)
	}

	return phase, index, nil
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

// ValidateAndMergeData validates submitted form data against this step and merges it into
// the provided configuration map. It performs three checks:
//  1. The stepId must match this step's Id.
//  2. The data must be valid JSON that passes the step's JSON Schema validation.
//  3. Only fields defined in the schema's top-level "properties" are merged into config,
//     preventing arbitrary data injection.
//
// The config map is modified in place. If config is nil, a new map is created and returned.
// Returns the (possibly new) config map and any validation error.
func (s *SetupFlowStep) ValidateAndMergeData(stepId string, data json.RawMessage, config map[string]any) (map[string]any, error) {
	if stepId == "" {
		return config, fmt.Errorf("step_id is required")
	}
	if stepId != s.Id {
		return config, fmt.Errorf("step_id %q does not match current step %q", stepId, s.Id)
	}

	// Parse the submitted data
	var submittedData map[string]any
	if err := json.Unmarshal(data, &submittedData); err != nil {
		return config, fmt.Errorf("invalid form data JSON: %w", err)
	}

	// Validate against JSON schema
	if err := validateDataAgainstSchema(data, json.RawMessage(s.JsonSchema)); err != nil {
		return config, fmt.Errorf("form data validation failed: %w", err)
	}

	// Extract allowed field names from the schema
	allowedFields, err := extractSchemaPropertyNames(json.RawMessage(s.JsonSchema))
	if err != nil {
		return config, fmt.Errorf("failed to parse schema properties: %w", err)
	}

	// Merge only allowed fields
	if config == nil {
		config = make(map[string]any)
	}
	for k, v := range submittedData {
		if allowedFields[k] {
			config[k] = v
		}
	}

	return config, nil
}

// validateDataAgainstSchema validates JSON data against a JSON Schema.
func validateDataAgainstSchema(data json.RawMessage, schema json.RawMessage) error {
	compiler := jsonschemav5.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader(schema)); err != nil {
		return fmt.Errorf("invalid JSON schema: %w", err)
	}

	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("failed to compile JSON schema: %w", err)
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON data: %w", err)
	}

	return compiled.Validate(v)
}

// extractSchemaPropertyNames parses a JSON Schema and returns the set of top-level
// property names defined in it.
func extractSchemaPropertyNames(schema json.RawMessage) (map[string]bool, error) {
	var parsed struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		return nil, err
	}

	result := make(map[string]bool, len(parsed.Properties))
	for k := range parsed.Properties {
		result[k] = true
	}
	return result, nil
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
