package rate_limit

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	jsonschemav5 "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
)

type schemaIdEnvelope struct {
	Id string `json:"$id"`
}

func loadSchemaInto(t *testing.T, c *jsonschemav5.Compiler, path string) string {
	schemaBytes, err := os.ReadFile(path)
	require.NoError(t, err)

	var sid schemaIdEnvelope
	require.NoError(t, json.Unmarshal(schemaBytes, &sid))
	require.NoError(t, c.AddResource(sid.Id, bytes.NewReader(schemaBytes)))
	return sid.Id
}

// TestSchemaId asserts the file-declared $id matches the Go-side constant
// so callers compiling by id resolve to the same artefact.
func TestSchemaId(t *testing.T) {
	c := jsonschemav5.NewCompiler()
	id := loadSchemaInto(t, c, "./schema.json")
	require.Equal(t, SchemaIdRateLimit, id)
}

// TestSchema runs each $def through the same valid/invalid table-test
// pattern used by internal/schema/common/schema_test.go.
func TestSchema(t *testing.T) {
	type testCase struct {
		Name  string
		Valid bool
		Data  string
	}

	type entity struct {
		Name   string
		Schema string
		Tests  []testCase
	}

	const testSchemaId = "https://raw.githubusercontent.com/rmorlok/authproxy/refs/heads/main/schema/resources/rate_limit/test.json"
	mkSchema := func(ref string) string {
		return strings.TrimSpace(`
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "` + testSchemaId + `",
  "type": "object",
  "additionalProperties": false,
  "required": ["test"],
  "properties": {
    "test": { "$ref": "` + ref + `" }
  }
}`)
	}

	entities := []entity{
		{
			Name:   "Mode",
			Schema: mkSchema("./schema.json#/$defs/Mode"),
			Tests: []testCase{
				{"enforce", true, `{"test": "enforce"}`},
				{"observe", true, `{"test": "observe"}`},
				{"empty rejected", false, `{"test": ""}`},
				{"unknown rejected", false, `{"test": "audit"}`},
				{"wrong type", false, `{"test": 1}`},
			},
		},
		{
			Name:   "PathMatch",
			Schema: mkSchema("./schema.json#/$defs/PathMatch"),
			Tests: []testCase{
				{"prefix ok", true, `{"test": {"kind": "prefix", "value": "/v1/"}}`},
				{"glob ok", true, `{"test": {"kind": "glob", "value": "/v1/*"}}`},
				{"regex ok", true, `{"test": {"kind": "regex", "value": "^/v1/[0-9]+$"}}`},
				{"missing kind", false, `{"test": {"value": "/v1/"}}`},
				{"missing value", false, `{"test": {"kind": "prefix"}}`},
				{"empty value", false, `{"test": {"kind": "prefix", "value": ""}}`},
				{"unknown kind", false, `{"test": {"kind": "wat", "value": "/v1/"}}`},
				{"extra prop", false, `{"test": {"kind": "prefix", "value": "/v1/", "extra": "x"}}`},
			},
		},
		{
			Name:   "Selector",
			Schema: mkSchema("./schema.json#/$defs/Selector"),
			Tests: []testCase{
				{"empty ok", true, `{"test": {}}`},
				{"label_selector only", true, `{"test": {"label_selector": "x=y"}}`},
				{"methods ok", true, `{"test": {"methods": ["GET", "POST"]}}`},
				{"unknown method", false, `{"test": {"methods": ["FROBNICATE"]}}`},
				{"path_match ok", true, `{"test": {"path_match": {"kind": "prefix", "value": "/x"}}}`},
				{"request_types ok", true, `{"test": {"request_types": ["proxy", "probe"]}}`},
				{"request_types empty rejected", false, `{"test": {"request_types": []}}`},
				{"request_types unknown rejected", false, `{"test": {"request_types": ["bogus"]}}`},
				{"extra prop rejected", false, `{"test": {"unknown": 1}}`},
			},
		},
		{
			Name:   "Bucket",
			Schema: mkSchema("./schema.json#/$defs/Bucket"),
			Tests: []testCase{
				{"empty ok", true, `{"test": {}}`},
				{"reserved ok", true, `{"test": {"dimensions": ["actor", "connection"]}}`},
				{"label key ok", true, `{"test": {"dimensions": ["labels/team"]}}`},
				{"unknown reserved rejected", false, `{"test": {"dimensions": ["team"]}}`},
				{"missing label key rejected", false, `{"test": {"dimensions": ["labels/"]}}`},
				{"duplicate rejected", false, `{"test": {"dimensions": ["actor", "actor"]}}`},
				{"extra prop rejected", false, `{"test": {"dimensions": [], "extra": 1}}`},
			},
		},
		{
			Name:   "FixedWindow",
			Schema: mkSchema("./schema.json#/$defs/FixedWindow"),
			Tests: []testCase{
				{"ok", true, `{"test": {"window": "1m", "limit": 10}}`},
				{"missing window", false, `{"test": {"limit": 10}}`},
				{"missing limit", false, `{"test": {"window": "1m"}}`},
				{"limit zero", false, `{"test": {"window": "1m", "limit": 0}}`},
				{"limit negative", false, `{"test": {"window": "1m", "limit": -1}}`},
				{"window invalid", false, `{"test": {"window": "blah", "limit": 1}}`},
			},
		},
		{
			Name:   "SlidingWindow",
			Schema: mkSchema("./schema.json#/$defs/SlidingWindow"),
			Tests: []testCase{
				{"log ok", true, `{"test": {"window": "1m", "limit": 10, "mode": "log"}}`},
				{"counter ok", true, `{"test": {"window": "1m", "limit": 10, "mode": "counter"}}`},
				{"missing mode", false, `{"test": {"window": "1m", "limit": 10}}`},
				{"unknown mode", false, `{"test": {"window": "1m", "limit": 10, "mode": "exact"}}`},
			},
		},
		{
			Name:   "TokenBucket",
			Schema: mkSchema("./schema.json#/$defs/TokenBucket"),
			Tests: []testCase{
				{"ok", true, `{"test": {"capacity": 10, "refill_rate": 1}}`},
				{"missing capacity", false, `{"test": {"refill_rate": 1}}`},
				{"missing refill_rate", false, `{"test": {"capacity": 10}}`},
				{"capacity zero", false, `{"test": {"capacity": 0, "refill_rate": 1}}`},
				{"refill_rate zero", false, `{"test": {"capacity": 10, "refill_rate": 0}}`},
			},
		},
		{
			Name:   "Algorithm",
			Schema: mkSchema("./schema.json#/$defs/Algorithm"),
			Tests: []testCase{
				{"none rejected", false, `{"test": {}}`},
				{"fixed_window only ok", true, `{"test": {"fixed_window": {"window": "1m", "limit": 10}}}`},
				{"sliding_window only ok", true, `{"test": {"sliding_window": {"window": "1m", "limit": 10, "mode": "log"}}}`},
				{"token_bucket only ok", true, `{"test": {"token_bucket": {"capacity": 5, "refill_rate": 0.5}}}`},
				{"two set rejected", false, `{"test": {"fixed_window": {"window": "1m", "limit": 1}, "token_bucket": {"capacity": 1, "refill_rate": 1}}}`},
				{"all set rejected", false, `{"test": {"fixed_window": {"window": "1m", "limit": 1}, "sliding_window": {"window": "1m", "limit": 1, "mode": "log"}, "token_bucket": {"capacity": 1, "refill_rate": 1}}}`},
				{"extra prop rejected", false, `{"test": {"fixed_window": {"window": "1m", "limit": 1}, "extra": 1}}`},
			},
		},
		{
			Name:   "RateLimit",
			Schema: mkSchema("./schema.json#/$defs/RateLimit"),
			Tests: []testCase{
				{
					"valid token_bucket",
					true,
					`{"test": {"selector": {"methods": ["GET"], "request_types": ["proxy"]}, "bucket": {"dimensions": ["actor"]}, "algorithm": {"token_bucket": {"capacity": 10, "refill_rate": 1}}}}`,
				},
				{
					"valid observe mode",
					true,
					`{"test": {"mode": "observe", "selector": {}, "bucket": {}, "algorithm": {"fixed_window": {"window": "1m", "limit": 10}}}}`,
				},
				{
					"missing selector",
					false,
					`{"test": {"bucket": {}, "algorithm": {"fixed_window": {"window": "1m", "limit": 10}}}}`,
				},
				{
					"missing algorithm",
					false,
					`{"test": {"selector": {}, "bucket": {}}}`,
				},
				{
					"unknown top-level prop rejected",
					false,
					`{"test": {"selector": {}, "bucket": {}, "algorithm": {"fixed_window": {"window": "1m", "limit": 10}}, "unknown": 1}}`,
				},
			},
		},
	}

	for _, e := range entities {
		t.Run(e.Name, func(t *testing.T) {
			c := jsonschemav5.NewCompiler()
			_ = loadSchemaInto(t, c, "../../common/schema.json")
			_ = loadSchemaInto(t, c, "./schema.json")
			require.NoError(t, c.AddResource(testSchemaId, strings.NewReader(e.Schema)))

			schema, err := c.Compile(testSchemaId)
			require.NoError(t, err)

			for _, tc := range e.Tests {
				t.Run(tc.Name, func(t *testing.T) {
					var v interface{}
					require.NoError(t, json.Unmarshal([]byte(tc.Data), &v))
					err := schema.Validate(v)
					if tc.Valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				})
			}
		})
	}
}
