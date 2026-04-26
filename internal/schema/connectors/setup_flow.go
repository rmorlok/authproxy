package connectors

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/go-multierror"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
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

// SetupStepPhase identifies which phase of the setup flow a step belongs to. The indexed
// phases (preconnect, configure) carry an integer index in their canonical setup-step form;
// the others are singleton pseudo-steps with no index.
type SetupStepPhase string

const (
	SetupPhasePreconnect   SetupStepPhase = "preconnect"
	SetupPhaseAuth         SetupStepPhase = "auth"
	SetupPhaseVerify       SetupStepPhase = "verify"
	SetupPhaseConfigure    SetupStepPhase = "configure"
	SetupPhaseVerifyFailed SetupStepPhase = "verify_failed"
	SetupPhaseAuthFailed   SetupStepPhase = "auth_failed"
)

// String returns the underlying phase identifier.
func (p SetupStepPhase) String() string { return string(p) }

// IsIndexed reports whether the phase carries a 0-based step index in its canonical form
// (preconnect, configure). Singleton pseudo-steps return false.
func (p SetupStepPhase) IsIndexed() bool {
	return p == SetupPhasePreconnect || p == SetupPhaseConfigure
}

// IsTerminalFailure reports whether the phase represents a terminal failure pseudo-step
// (verify_failed, auth_failed). Connections in this phase are retryable via the retry endpoint.
func (p SetupStepPhase) IsTerminalFailure() bool {
	return p == SetupPhaseVerifyFailed || p == SetupPhaseAuthFailed
}

// IsValid reports whether p is one of the recognized phases.
func (p SetupStepPhase) IsValid() bool {
	switch p {
	case SetupPhasePreconnect, SetupPhaseAuth, SetupPhaseVerify, SetupPhaseConfigure,
		SetupPhaseVerifyFailed, SetupPhaseAuthFailed:
		return true
	}
	return false
}

// SetupStep is a typed representation of a setup-flow step. It captures the phase and, for
// indexed phases, the 0-based step index. The zero SetupStep is invalid; use ParseSetupStep,
// NewSetupStep, or NewIndexedSetupStep to construct one.
type SetupStep struct {
	phase SetupStepPhase
	index int
}

// NewSetupStep returns a SetupStep for a singleton (non-indexed) phase.
// Returns an error if phase is indexed or unknown.
func NewSetupStep(phase SetupStepPhase) (SetupStep, error) {
	if !phase.IsValid() {
		return SetupStep{}, fmt.Errorf("unknown setup phase %q", phase)
	}
	if phase.IsIndexed() {
		return SetupStep{}, fmt.Errorf("phase %q is indexed; use NewIndexedSetupStep", phase)
	}
	return SetupStep{phase: phase}, nil
}

// NewIndexedSetupStep returns a SetupStep for an indexed phase (preconnect, configure).
// Returns an error if phase is not indexed or index is negative.
func NewIndexedSetupStep(phase SetupStepPhase, index int) (SetupStep, error) {
	if !phase.IsIndexed() {
		return SetupStep{}, fmt.Errorf("phase %q is not indexed", phase)
	}
	if index < 0 {
		return SetupStep{}, fmt.Errorf("setup step index must be non-negative, got %d", index)
	}
	return SetupStep{phase: phase, index: index}, nil
}

// MustNewIndexedSetupStep is like NewIndexedSetupStep but panics on error.
func MustNewIndexedSetupStep(phase SetupStepPhase, index int) SetupStep {
	step, err := NewIndexedSetupStep(phase, index)
	if err != nil {
		panic(err)
	}
	return step
}

// Predefined SetupSteps for the singleton phases.
var (
	SetupStepAuth         = SetupStep{phase: SetupPhaseAuth}
	SetupStepVerify       = SetupStep{phase: SetupPhaseVerify}
	SetupStepVerifyFailed = SetupStep{phase: SetupPhaseVerifyFailed}
	SetupStepAuthFailed   = SetupStep{phase: SetupPhaseAuthFailed}
)

// Phase returns the step's phase.
func (s SetupStep) Phase() SetupStepPhase { return s.phase }

// Index returns the 0-based index for indexed phases. Returns 0 for singleton phases.
func (s SetupStep) Index() int { return s.index }

// String renders the canonical setup-step form: "phase:index" for indexed phases, "phase"
// for singletons. The zero SetupStep returns "".
func (s SetupStep) String() string {
	if s.phase == "" {
		return ""
	}
	if s.phase.IsIndexed() {
		return fmt.Sprintf("%s:%d", s.phase, s.index)
	}
	return string(s.phase)
}

// IsZero reports whether s is the zero SetupStep (no phase set).
func (s SetupStep) IsZero() bool { return s.phase == "" }

// Equals reports whether s and other are identical.
func (s SetupStep) Equals(other SetupStep) bool { return s == other }

// IsTerminalFailure reports whether the step is in a terminal failure phase.
func (s SetupStep) IsTerminalFailure() bool { return s.phase.IsTerminalFailure() }

// Value implements driver.Valuer so SetupStep can be stored as a database column.
// The zero SetupStep round-trips as SQL NULL.
func (s SetupStep) Value() (driver.Value, error) {
	if s.IsZero() {
		return nil, nil
	}
	return s.String(), nil
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

	if str == "" {
		*s = SetupStep{}
		return nil
	}

	parsed, err := ParseSetupStep(str)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}

// ParseSetupStep parses a setup-step string into a SetupStep. Indexed forms must be
// "phase:index" (e.g. "preconnect:0", "configure:3"); singleton phases ("auth", "verify",
// "verify_failed", "auth_failed") parse as a SetupStep with index 0.
func ParseSetupStep(setupStep string) (SetupStep, error) {
	phase := SetupStepPhase(setupStep)
	if phase.IsValid() && !phase.IsIndexed() {
		return SetupStep{phase: phase}, nil
	}

	parts := strings.SplitN(setupStep, ":", 2)
	if len(parts) != 2 {
		return SetupStep{}, fmt.Errorf("invalid setup step format %q", setupStep)
	}

	phase = SetupStepPhase(parts[0])
	if !phase.IsIndexed() {
		return SetupStep{}, fmt.Errorf("invalid setup step phase %q", phase)
	}

	index, err := strconv.Atoi(parts[1])
	if err != nil {
		return SetupStep{}, fmt.Errorf("invalid setup step index %q: %w", parts[1], err)
	}

	if index < 0 {
		return SetupStep{}, fmt.Errorf("setup step index must be non-negative, got %d", index)
	}

	return SetupStep{phase: phase, index: index}, nil
}

// GetStepBySetupStep returns the step definition and its 0-based global index for the given
// setup step. Returns an error if step is not an indexed phase or its index is out of range.
func (sf *SetupFlow) GetStepBySetupStep(step SetupStep) (*SetupFlowStep, int, error) {
	switch step.phase {
	case SetupPhasePreconnect:
		if sf.Preconnect == nil || step.index >= len(sf.Preconnect.Steps) {
			return nil, 0, fmt.Errorf("preconnect step index %d out of range", step.index)
		}
		return &sf.Preconnect.Steps[step.index], step.index, nil
	case SetupPhaseConfigure:
		if sf.Configure == nil || step.index >= len(sf.Configure.Steps) {
			return nil, 0, fmt.Errorf("configure step index %d out of range", step.index)
		}
		globalIndex := step.index
		if sf.Preconnect != nil {
			globalIndex += len(sf.Preconnect.Steps)
		}
		return &sf.Configure.Steps[step.index], globalIndex, nil
	default:
		return nil, 0, fmt.Errorf("phase %q does not have indexed steps", step.phase)
	}
}

// FirstSetupStep returns the first step in the flow. Returns the zero SetupStep when the
// flow has no steps (caller should check IsZero).
func (sf *SetupFlow) FirstSetupStep() SetupStep {
	if sf == nil {
		return SetupStep{}
	}
	if sf.HasPreconnect() {
		return SetupStep{phase: SetupPhasePreconnect}
	}
	if sf.HasConfigure() {
		return SetupStep{phase: SetupPhaseConfigure}
	}
	return SetupStep{}
}

// NextSetupStep returns the step that follows current. The auth phase is implicit between
// preconnect and configure phases; when the connector has probes, a verify phase runs between
// auth and configure. Returns the zero SetupStep (IsZero) when current is the final step.
func (sf *SetupFlow) NextSetupStep(current SetupStep, hasProbes bool) (SetupStep, error) {
	switch current.phase {
	case SetupPhasePreconnect:
		if sf.Preconnect != nil && current.index+1 < len(sf.Preconnect.Steps) {
			return SetupStep{phase: SetupPhasePreconnect, index: current.index + 1}, nil
		}
		// Preconnect done — next is auth
		return SetupStepAuth, nil
	case SetupPhaseAuth:
		if hasProbes {
			return SetupStepVerify, nil
		}
		if sf.HasConfigure() {
			return SetupStep{phase: SetupPhaseConfigure}, nil
		}
		return SetupStep{}, nil // Complete
	case SetupPhaseVerify:
		if sf.HasConfigure() {
			return SetupStep{phase: SetupPhaseConfigure}, nil
		}
		return SetupStep{}, nil // Complete
	case SetupPhaseConfigure:
		if sf.Configure != nil && current.index+1 < len(sf.Configure.Steps) {
			return SetupStep{phase: SetupPhaseConfigure, index: current.index + 1}, nil
		}
		return SetupStep{}, nil // Complete
	default:
		return SetupStep{}, fmt.Errorf("no successor defined for phase %q", current.phase)
	}
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
