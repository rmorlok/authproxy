package config

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestToolInvocationLimitsSerialization exercises ordinary JSON/YAML codecs for
// every configured field, including the shared human duration and byte types.
func TestToolInvocationLimitsSerialization(t *testing.T) {
	const source = `tools:
  invocation:
    timeout: 7s
    maxRequests: 3
    maxInputBytes: 2mib
    maxResponseBytes: 4mib
    maxOutputBytes: 1mib
    maxEncodedResultBytes: 8mib
    maxStackDepth: 100
    maxRenderBytes: 16mib
    maxTransformInputBytes: 32mib
    maxResponseStreamBytes: 64mib
    maxResponseHeaderBytes: 32kib
    maxResponseSourceBytes: 128kib
    maxDepth: 32
    maxValues: 50000
    maxRenderSteps: 90000
    maxNumberDigits: 512
`
	// Both native decoders produce the same complete configuration contract.
	var fromYAML, fromJSON Root
	require.NoError(t, util.DecodeYAMLStrict([]byte(source), &fromYAML))
	jsonSource, err := util.YamlBytesToJSON([]byte(source))
	require.NoError(t, err)
	require.NoError(t, util.DecodeJSONStrict(jsonSource, &fromJSON))
	require.Equal(t, fromYAML.Tools, fromJSON.Tools)
	require.NoError(t, fromJSON.Tools.Validate(nil))
	l := fromJSON.Tools.Invocation
	require.Equal(t, 7*time.Second, l.Timeout.Duration)
	require.Equal(t, 3, *l.MaxRequests)
	require.Equal(t, 100, *l.MaxStackDepth)
	require.Equal(t, 32, *l.MaxDepth)
	require.Equal(t, 50000, *l.MaxValues)
	require.Equal(t, 90000, *l.MaxRenderSteps)
	require.Equal(t, 512, *l.MaxNumberDigits)
	for _, field := range []struct {
		value *HumanByteSize
		bytes uint64
	}{
		{l.MaxInputBytes, 2 * common.MiB},
		{l.MaxResponseBytes, 4 * common.MiB},
		{l.MaxOutputBytes, common.MiB},
		{l.MaxEncodedResultBytes, 8 * common.MiB},
		{l.MaxRenderBytes, 16 * common.MiB},
		{l.MaxTransformInputBytes, 32 * common.MiB},
		{l.MaxResponseStreamBytes, 64 * common.MiB},
		{l.MaxResponseHeaderBytes, 32 * common.KiB},
		{l.MaxResponseSourceBytes, 128 * common.KiB},
	} {
		require.Equal(t, field.bytes, field.value.Value())
	}

	// Marshaling either syntax retains all overrides without a custom wrapper.
	jsonRoundTrip, err := json.Marshal(fromJSON.Tools)
	require.NoError(t, err)
	var jsonTools Tools
	require.NoError(t, json.Unmarshal(jsonRoundTrip, &jsonTools))
	require.Equal(t, fromJSON.Tools, &jsonTools)
	yamlRoundTrip, err := yaml.Marshal(fromYAML.Tools)
	require.NoError(t, err)
	var yamlTools Tools
	require.NoError(t, yaml.Unmarshal(yamlRoundTrip, &yamlTools))
	require.Equal(t, fromYAML.Tools, &yamlTools)
}

// TestToolInvocationLimitsValidate checks optionality and every configured
// ceiling, including machine-sized byte conversion and rational number bounds.
func TestToolInvocationLimitsValidate(t *testing.T) {
	// Absence selects runtime defaults without modifying the schema object.
	require.NoError(t, (*Tools)(nil).Validate(nil))
	require.NoError(t, (*ToolInvocationLimits)(nil).Validate(nil))
	empty := &Tools{Invocation: &ToolInvocationLimits{}}
	require.NoError(t, empty.Validate(nil))
	encoded, err := json.Marshal(empty)
	require.NoError(t, err)
	require.JSONEq(t, `{"invocation":{}}`, string(encoded))

	counts := []string{"maxRequests", "maxStackDepth", "maxDepth", "maxValues", "maxRenderSteps", "maxNumberDigits"}
	bytes := []string{"maxInputBytes", "maxResponseBytes", "maxOutputBytes", "maxEncodedResultBytes", "maxRenderBytes", "maxTransformInputBytes", "maxResponseStreamBytes", "maxResponseHeaderBytes", "maxResponseSourceBytes"}
	for _, field := range counts {
		for _, value := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", field, value), func(t *testing.T) {
				// Explicit nonpositive counts fail at their full configuration path.
				var tools Tools
				require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"invocation":{"%s":%d}}`, field, value)), &tools))
				err := tools.Validate(&common.ValidationContext{Path: "$.tools"})
				if value > 0 {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "$.tools.invocation."+field)
				}
			})
		}
	}
	// HumanByteSize's existing parser uses floating-point conversion. Use an
	// exactly representable value below MaxInt rather than a decimal that rounds up.
	nearMaxInt := uint64(math.MaxInt) / 2048 * 2048
	for _, field := range bytes {
		for _, value := range []uint64{0, 1, nearMaxInt, uint64(math.MaxInt) + 1} {
			t.Run(fmt.Sprintf("%s/%d", field, value), func(t *testing.T) {
				// Byte strings must remain representable before conversion to int.
				var tools Tools
				require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"invocation":{"%s":"%db"}}`, field, value)), &tools))
				err := tools.Validate(&common.ValidationContext{Path: "$.tools"})
				if value > 0 && value <= uint64(math.MaxInt) {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "$.tools.invocation."+field)
				}
			})
		}
	}

	// Programmatic durations also reject negatives, independent of text decoding.
	for _, value := range []time.Duration{-time.Second, 0, time.Nanosecond} {
		limits := &ToolInvocationLimits{Timeout: &HumanDuration{Duration: value}}
		if value > 0 {
			require.NoError(t, limits.Validate(nil))
		} else {
			require.ErrorContains(t, limits.Validate(nil), "$.timeout")
		}
	}
	require.NoError(t, (&ToolInvocationLimits{MaxNumberDigits: util.ToPtr(4096)}).Validate(nil))
	require.ErrorContains(t, (&ToolInvocationLimits{MaxNumberDigits: util.ToPtr(4097)}).Validate(nil), "$.maxNumberDigits")

	// Root validation includes Tool errors alongside other required configuration.
	root := &Root{Tools: &Tools{Invocation: &ToolInvocationLimits{MaxRequests: util.ToPtr(0)}}}
	require.ErrorContains(t, root.Validate(), "$.tools.invocation.maxRequests")
}

// TestToolsRejectUnknownFields preserves strict config loading at both new
// nesting levels without custom JSON or YAML deserialization methods.
func TestToolsRejectUnknownFields(t *testing.T) {
	for _, source := range []string{
		`{"tools":{"unknown":true}}`,
		`{"tools":{"invocation":{"unknown":true}}}`,
		`{"tools":{"invocation":{"max_requests":3}}}`,
	} {
		// The standard strict utilities cover JSON and JSON-compatible YAML input.
		var root Root
		require.Error(t, util.DecodeJSONStrict([]byte(source), &root))
		require.Error(t, util.DecodeYAMLStrict([]byte(source), &root))
	}
}

// TestToolsSchema verifies the published configuration shape using the actual
// root schema and its dependencies rather than a standalone test projection.
func TestToolsSchema(t *testing.T) {
	c := jsonschemav5.NewCompiler()
	for _, path := range []string{
		"../resources/namespace/schema.json", "../resources/meta/schema.json", "../auth/schema.json",
		"../common/schema.json", "../resources/connectors/schema-oauth.json", "../resources/connectors/schema.json",
		"../resources/key/schema.json", "../resources/actor/schema.json", "../resources/rate_limit/schema.json",
	} {
		loadSchema(t, c, path)
	}
	schema, err := c.Compile(loadSchema(t, c, "./schema.json"))
	require.NoError(t, err)

	// Optional blocks and positive overrides use the same camelCase contract.
	for _, source := range []string{
		`{}`, `{"tools":{}}`, `{"tools":{"invocation":{}}}`,
		`{"tools":{"invocation":{"timeout":"3s","maxRequests":3,"maxInputBytes":"1mib","maxResponseBytes":"8mib","maxOutputBytes":"8mib","maxEncodedResultBytes":"16mib","maxStackDepth":256,"maxRenderBytes":"8mib","maxTransformInputBytes":"16mib","maxResponseStreamBytes":"8mib","maxResponseHeaderBytes":"64kib","maxResponseSourceBytes":"256kib","maxDepth":64,"maxValues":1000000,"maxRenderSteps":1000000,"maxNumberDigits":4096}}}`,
	} {
		var value any
		require.NoError(t, json.Unmarshal([]byte(source), &value))
		require.NoError(t, schema.Validate(value), source)
	}

	// Reject unknown keys, nonpositive counts, and incorrect scalar types.
	for _, source := range []string{
		`{"tools":{"unknown":true}}`,
		`{"tools":{"invocation":{"unknown":true}}}`,
		`{"tools":{"invocation":{"maxRequests":0}}}`,
		`{"tools":{"invocation":{"maxNumberDigits":4097}}}`,
		`{"tools":{"invocation":{"maxInputBytes":1024}}}`,
		`{"tools":{"invocation":{"timeout":60}}}`,
		`{"tools":{"invocation":{"maxStackDepth":"256"}}}`,
	} {
		var value any
		require.NoError(t, json.Unmarshal([]byte(source), &value))
		require.Error(t, schema.Validate(value), source)
	}
}
