package connectors

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// SetupFlow defines the user-authored portion of a connector's setup flow.
// Auth-method-emitted credential steps (api-key form, OAuth2 redirect) are
// not described here — they are materialized at runtime by the auth method
// and inserted between preconnect and configure by the core flow builder.
type SetupFlow struct {
	// Preconnect defines form steps shown before any credential acquisition. Values
	// collected here are available for mustache templating in auth configuration
	// (e.g. tenant subdomain in OAuth endpoints).
	Preconnect *SetupFlowPhase `json:"preconnect,omitempty" yaml:"preconnect,omitempty"`

	// Configure defines form steps shown after credentials are established.
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

// TotalSteps returns the total number of user-authored form steps in the
// schema (preconnect + configure). Excludes auth-method-emitted steps — the
// runtime ManifestSetupFlow counts those separately.
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

// ApxyStepPrefix is the reserved prefix for system-emitted step ids. User-
// authored step ids must not start with this prefix. Auth-method-emitted
// steps use "apxy:auth:<val>" (e.g. "apxy:auth:oauth2_authorize");
// terminal pseudo-steps use "apxy:verify_failed" / "apxy:auth_failed";
// the verify pseudo-step is "apxy:verify".
const ApxyStepPrefix = "apxy:"

// SetupStep is the typed identifier for a step within a connection's setup
// flow. It is the value stored in connections.setup_step_id and threaded
// through the API surface. User-authored steps carry the id from the
// connector YAML; system-emitted steps use the apxy: prefix.
type SetupStep struct {
	id string
}

// NewSetupStep constructs a SetupStep from an id string. The empty id is
// rejected; use the zero SetupStep (i.e. omit the field) for "no active step."
func NewSetupStep(id string) (SetupStep, error) {
	if id == "" {
		return SetupStep{}, fmt.Errorf("setup step id is required")
	}
	return SetupStep{id: id}, nil
}

// MustNewSetupStep is like NewSetupStep but panics on error. Useful for
// constructing predefined singleton steps at package init time.
func MustNewSetupStep(id string) SetupStep {
	step, err := NewSetupStep(id)
	if err != nil {
		panic(err)
	}
	return step
}

// Predefined SetupSteps for the system-emitted pseudo-steps the schema
// itself owns.
//
//   - SetupStepVerify is the post-auth "probes running" pseudo-step.
//   - SetupStepVerifyFailed and SetupStepAuthFailed are terminal failure
//     pseudo-steps; connections in these are retryable via the retry endpoint.
//
// Auth-method-emitted steps (e.g. apxy:auth:oauth2_authorize) are not
// predefined here — they're constructed in the respective auth_methods/*
// packages and exposed as package-level constants there.
var (
	SetupStepVerify       = MustNewSetupStep(ApxyStepPrefix + "verify")
	SetupStepVerifyFailed = MustNewSetupStep(ApxyStepPrefix + "verify_failed")
	SetupStepAuthFailed   = MustNewSetupStep(ApxyStepPrefix + "auth_failed")
)

// Id returns the underlying id string.
func (s SetupStep) Id() string { return s.id }

// String renders the step id. The zero SetupStep returns "".
func (s SetupStep) String() string { return s.id }

// IsZero reports whether s is the zero SetupStep (no id set).
func (s SetupStep) IsZero() bool { return s.id == "" }

// Equals reports whether s and other are identical.
func (s SetupStep) Equals(other SetupStep) bool { return s.id == other.id }

// IsTerminalFailure reports whether the step is in a terminal failure state
// (verify_failed or auth_failed) — connections in this state are retryable
// via the retry endpoint.
func (s SetupStep) IsTerminalFailure() bool {
	return s.Equals(SetupStepVerifyFailed) || s.Equals(SetupStepAuthFailed)
}

// IsApxyEmitted reports whether the step id is system-emitted (i.e. carries
// the reserved apxy: prefix). User-authored steps return false.
func (s SetupStep) IsApxyEmitted() bool {
	return strings.HasPrefix(s.id, ApxyStepPrefix)
}

// Value implements driver.Valuer so SetupStep can be stored as a database column.
// The zero SetupStep round-trips as SQL NULL.
func (s SetupStep) Value() (driver.Value, error) {
	if s.IsZero() {
		return nil, nil
	}
	return s.id, nil
}

// Scan implements sql.Scanner so SetupStep can be read from a database column.
// NULL and empty values produce the zero SetupStep.
func (s *SetupStep) Scan(value interface{}) error {
	if value == nil {
		*s = SetupStep{}
		return nil
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("cannot scan %T into SetupStep", value)
	}

	*s = SetupStep{id: str}
	return nil
}

// MarshalJSON renders SetupStep as the step id string. The zero SetupStep
// marshals as JSON null so it round-trips cleanly with omitempty pointer
// fields and database NULL.
func (s SetupStep) MarshalJSON() ([]byte, error) {
	if s.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(s.id)
}

// UnmarshalJSON parses a SetupStep from a JSON string, accepting null and the
// empty string as the zero SetupStep.
func (s *SetupStep) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*s = SetupStep{}
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("setup step must be a JSON string: %w", err)
	}

	*s = SetupStep{id: str}
	return nil
}

// MarshalYAML renders SetupStep as the step id string. The zero SetupStep
// marshals as YAML null.
func (s SetupStep) MarshalYAML() (interface{}, error) {
	if s.IsZero() {
		return nil, nil
	}
	return s.id, nil
}

// UnmarshalYAML parses a SetupStep from a YAML scalar, accepting null and the
// empty string as the zero SetupStep.
func (s *SetupStep) UnmarshalYAML(value *yaml.Node) error {
	if value == nil || value.Tag == "!!null" {
		*s = SetupStep{}
		return nil
	}

	var str string
	if err := value.Decode(&str); err != nil {
		return fmt.Errorf("setup step must be a YAML string: %w", err)
	}

	*s = SetupStep{id: str}
	return nil
}

// ParseSetupStep parses a setup-step string into a SetupStep. The empty
// string returns the zero SetupStep.
func ParseSetupStep(setupStep string) (SetupStep, error) {
	if setupStep == "" {
		return SetupStep{}, nil
	}
	return SetupStep{id: setupStep}, nil
}

// IsConfigureStep returns true when the supplied step belongs to the
// schema's configure phase. Data sources are only available during configure
// steps (preconnect cannot use them because credentials are not yet
// established); callers use this to gate proxy-driven data-source requests.
func (sf *SetupFlow) IsConfigureStep(step SetupStep) bool {
	if sf == nil || sf.Configure == nil || step.IsZero() {
		return false
	}
	for i := range sf.Configure.Steps {
		if sf.Configure.Steps[i].Id == step.id {
			return true
		}
	}
	return false
}

// FindStepById returns the user-authored step definition with the given id
// from preconnect or configure. ok=false if the id does not match any
// schema-defined step (e.g. it's an apxy:* pseudo-step or an
// auth-method-emitted step id).
func (sf *SetupFlow) FindStepById(id string) (step *SetupFlowStep, ok bool) {
	if sf == nil || id == "" {
		return nil, false
	}
	if sf.Preconnect != nil {
		for i := range sf.Preconnect.Steps {
			if sf.Preconnect.Steps[i].Id == id {
				return &sf.Preconnect.Steps[i], true
			}
		}
	}
	if sf.Configure != nil {
		for i := range sf.Configure.Steps {
			if sf.Configure.Steps[i].Id == id {
				return &sf.Configure.Steps[i], true
			}
		}
	}
	return nil, false
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

// SetupFlowStepType identifies the runtime kind of a user-authored setup step.
// Defaults to form when empty.
type SetupFlowStepType string

const (
	// SetupFlowStepTypeForm is a JSONForms-rendered form the user fills out.
	// JsonSchema is required; Redirect must be absent.
	SetupFlowStepTypeForm SetupFlowStepType = "form"

	// SetupFlowStepTypeRedirect sends the user off-platform to a 3rd party,
	// optionally with a templated URL that includes signed RETURN_ADVANCE /
	// RETURN_ABORT tokens for the 3rd party to bounce the user back through.
	// Redirect is required; JsonSchema / UiSchema / DataSources must be absent.
	SetupFlowStepTypeRedirect SetupFlowStepType = "redirect"
)

// IsValid reports whether t is one of the recognized step kinds. The zero
// value ("") is treated as valid by callers that default to form.
func (t SetupFlowStepType) IsValid() bool {
	return t == "" || t == SetupFlowStepTypeForm || t == SetupFlowStepTypeRedirect
}

// Normalized returns the effective type — form if empty, otherwise t.
func (t SetupFlowStepType) Normalized() SetupFlowStepType {
	if t == "" {
		return SetupFlowStepTypeForm
	}
	return t
}

// SetupFlowStep defines a single step in the setup flow. The Type field
// selects which of the per-kind sub-structs (currently just Redirect) is
// honored; form-kind steps populate JsonSchema/UiSchema/DataSources
// directly on the parent.
type SetupFlowStep struct {
	// Id is a unique identifier for this step within the connector's setup flow.
	// Must not start with the apxy: prefix, which is reserved for system-emitted
	// steps.
	Id string `json:"id" yaml:"id"`

	// Type selects the step kind. Defaults to form when empty.
	Type SetupFlowStepType `json:"type,omitempty" yaml:"type,omitempty"`

	// Title is the human-readable title shown above the form / redirect notice.
	Title string `json:"title,omitempty" yaml:"title,omitempty"`

	// Description is additional explanatory text shown with the step.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// JsonSchema is the JSON Schema defining the form's data model and
	// validation rules. Required for form-kind steps; must be absent for
	// redirect-kind steps.
	JsonSchema common.RawJSON `json:"json_schema,omitempty" yaml:"json_schema,omitempty"`

	// UiSchema is the JSONForms UI Schema defining the form's layout and
	// rendering. Optional for form steps; must be absent for redirect steps.
	UiSchema common.RawJSON `json:"ui_schema,omitempty" yaml:"ui_schema,omitempty"`

	// DataSources defines dynamic data endpoints that can be referenced by form fields
	// using the x-data-source property in the JSON Schema. Only allowed in configure
	// form steps.
	DataSources map[string]DataSourceDef `json:"data_sources,omitempty" yaml:"data_sources,omitempty"`

	// If optionally gates this user-authored setup step. Runtime evaluates the
	// configured condition server-side; clients only see eligible steps.
	If *common.Predicate `json:"if,omitempty" yaml:"if,omitempty"`

	// Redirect carries the redirect-step-specific configuration. Required
	// when Type == redirect; must be absent otherwise.
	Redirect *SetupFlowStepRedirect `json:"redirect,omitempty" yaml:"redirect,omitempty"`
}

// SetupFlowStepRedirect describes a redirect-kind step's destination. The
// URL is a mustache template that supports the connection's cfg fields
// (e.g. {{cfg.tenant}}) plus two synthetic placeholders the runtime
// substitutes at render time:
//
//   - {{RETURN_ADVANCE}}: a signed one-time-use URL the 3rd party redirects
//     the user back to after the off-platform step succeeds; consuming it
//     advances the connection to the next setup step.
//   - {{RETURN_ABORT}}: same shape, used when the user cancels; consuming
//     it aborts the in-flight setup.
//
// Tokens are signed with the AuthProxy instance's system signing key and
// tracked in Redis for one-time-use enforcement.
type SetupFlowStepRedirect struct {
	// URL is the redirect destination. Mustache-templated; required.
	URL string `json:"url" yaml:"url"`
}

// Validate the redirect block in isolation. Cross-field validation
// (Redirect must be absent for form steps, present for redirect steps)
// happens at the SetupFlowStep level.
func (r *SetupFlowStepRedirect) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}
	if r.URL == "" {
		result = multierror.Append(result, vc.NewErrorfForField("url", "url is required"))
	}
	return result.ErrorOrNil()
}

func (s *SetupFlowStep) Validate(vc *common.ValidationContext, seenIds map[string]bool, allowDataSources bool) error {
	result := &multierror.Error{}

	if s.Id == "" {
		result = multierror.Append(result, vc.NewErrorfForField("id", "step id is required"))
	} else if strings.HasPrefix(s.Id, ApxyStepPrefix) {
		result = multierror.Append(result, vc.NewErrorfForField("id", "step id %q must not start with reserved prefix %q", s.Id, ApxyStepPrefix))
	} else if seenIds[s.Id] {
		result = multierror.Append(result, vc.NewErrorfForField("id", "duplicate step id %q", s.Id))
	} else {
		seenIds[s.Id] = true
	}

	if !s.Type.IsValid() {
		result = multierror.Append(result, vc.NewErrorfForField("type", "type must be %q or %q (got %q)", SetupFlowStepTypeForm, SetupFlowStepTypeRedirect, s.Type))
	}

	if err := s.If.Validate(vc.PushField("if"), connectorPredicateValidationVars()); err != nil {
		result = multierror.Append(result, err)
	}

	switch s.Type.Normalized() {
	case SetupFlowStepTypeForm:
		if s.Redirect != nil {
			result = multierror.Append(result, vc.NewErrorfForField("redirect", "redirect must be absent for form steps"))
		}
		if s.JsonSchema.IsEmpty() {
			result = multierror.Append(result, vc.NewErrorfForField("json_schema", "json_schema is required for form steps"))
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

	case SetupFlowStepTypeRedirect:
		if !s.JsonSchema.IsEmpty() {
			result = multierror.Append(result, vc.NewErrorfForField("json_schema", "json_schema must be absent for redirect steps"))
		}
		if !s.UiSchema.IsEmpty() {
			result = multierror.Append(result, vc.NewErrorfForField("ui_schema", "ui_schema must be absent for redirect steps"))
		}
		if len(s.DataSources) > 0 {
			result = multierror.Append(result, vc.NewErrorfForField("data_sources", "data_sources must be absent for redirect steps"))
		}
		if s.Redirect == nil {
			result = multierror.Append(result, vc.NewErrorfForField("redirect", "redirect is required for redirect steps"))
		} else {
			if err := s.Redirect.Validate(vc.PushField("redirect")); err != nil {
				result = multierror.Append(result, err)
			}
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

// PreconnectFieldNames returns the union of top-level property names defined across
// all preconnect steps' JSON schemas. These are the cfg fields available before the
// auth flow runs. Steps with malformed JSON schemas are silently skipped — those are
// caught by the per-step Validate.
func (sf *SetupFlow) PreconnectFieldNames() map[string]bool {
	result := make(map[string]bool)
	if sf == nil || sf.Preconnect == nil {
		return result
	}
	for i := range sf.Preconnect.Steps {
		step := &sf.Preconnect.Steps[i]
		if step.JsonSchema.IsEmpty() {
			continue
		}
		names, err := extractSchemaPropertyNames(json.RawMessage(step.JsonSchema))
		if err != nil {
			continue
		}
		for k := range names {
			result[k] = true
		}
	}
	return result
}

// ConfigureFieldNamesUpTo returns the union of top-level property names defined by
// configure steps with index < stepIndex. Used to determine which cfg fields are
// available to a data source rendered while the user is filling out configure step N.
// Pass len(steps) to get all configure fields.
func (sf *SetupFlow) ConfigureFieldNamesUpTo(stepIndex int) map[string]bool {
	result := make(map[string]bool)
	if sf == nil || sf.Configure == nil {
		return result
	}
	limit := stepIndex
	if limit > len(sf.Configure.Steps) {
		limit = len(sf.Configure.Steps)
	}
	for i := 0; i < limit; i++ {
		step := &sf.Configure.Steps[i]
		if step.JsonSchema.IsEmpty() {
			continue
		}
		names, err := extractSchemaPropertyNames(json.RawMessage(step.JsonSchema))
		if err != nil {
			continue
		}
		for k := range names {
			result[k] = true
		}
	}
	return result
}

// AllConfigFieldNames returns the union of cfg fields available after the entire
// setup flow completes (preconnect + all configure steps).
func (sf *SetupFlow) AllConfigFieldNames() map[string]bool {
	return util.UnionBoolMaps(
		sf.PreconnectFieldNames(),
		sf.ConfigureFieldNamesUpTo(1<<31-1),
	)
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

// ValidateMustacheReferences checks that every {{cfg.X}} reference in the URL and
// header templates resolves against the cfg fields visible to a configure step at
// the given index. Visibility = preconnect fields plus any prior configure step's
// fields (the step's own fields are not yet committed when the data source runs).
func (r *DataSourceProxyRequest) ValidateMustacheReferences(
	vc *common.ValidationContext,
	mctx *MustacheValidationContext,
	stepIdx int,
) error {
	if r == nil || mctx == nil {
		return nil
	}

	result := &multierror.Error{}
	available := mctx.ConfigureStepFields(stepIdx)
	scopeLabel := configureScopeLabel(stepIdx)

	checkMustacheTemplate(vc.PushField("url"), r.Url, available, scopeLabel, result)
	for k, v := range r.Headers {
		checkMustacheTemplate(vc.PushField("headers").PushField(k), v, available, scopeLabel, result)
	}

	return result.ErrorOrNil()
}
