package schema

import (
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"strings"
	"sync"
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/api"
	apiV1Alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

var allSchemaIDs = []string{
	api.SchemaIdAPI,
	apiV1Alpha1.SchemaID,
	auth.SchemaIdAuth,
	common.SchemaIdCommon,
	config.SchemaIdConfig,
	actor.SchemaIdActor,
	connectors.SchemaIdConnectors,
	meta.SchemaID,
	namespace.SchemaId,
	rate_limit.SchemaIdRateLimit,
}

func Test_AllSchemasCompile(t *testing.T) {
	for _, schemaId := range allSchemaIDs {
		_, err := CompileSchema(schemaId)
		require.NoError(t, err, "schema %s should compile", schemaId)
	}
}

func TestCompileSchemaConcurrentCacheMisses(t *testing.T) {
	require.NoError(t, loadSchemasOnce())
	// Use a fresh compiler so this test exercises cold compilation regardless of
	// which embedded schemas earlier tests compiled. These tests are not parallel.
	compiler := jsonschema.NewCompiler()
	const base = "https://schema.test/"
	require.NoError(t, compiler.AddResource(base+"shared", strings.NewReader(`{"$defs":{"value":{"type":"string","minLength":1}}}`)))
	for _, name := range []string{"first", "second"} {
		require.NoError(t, compiler.AddResource(base+name, strings.NewReader(`{"type":"object","properties":{"value":{"$ref":"shared#/$defs/value"}},"required":["value"]}`)))
	}
	compileMutex.Lock()
	previousCompiler, previousCache := schemaCompiler, schemaCache
	schemaCompiler, schemaCache = compiler, make(map[string]*jsonschema.Schema)
	compileMutex.Unlock()
	t.Cleanup(func() {
		compileMutex.Lock()
		schemaCompiler, schemaCache = previousCompiler, previousCache
		compileMutex.Unlock()
	})

	const workers = 32
	type result struct {
		id     string
		schema *jsonschema.Schema
		err    error
	}
	results := make(chan result, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id := base + []string{"first", "second"}[i%2]
			compiled, err := CompileSchema(id)
			results <- result{id, compiled, err}
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	seen := map[string]*jsonschema.Schema{}
	for result := range results {
		require.NoError(t, result.err)
		require.NoError(t, result.schema.Validate(map[string]any{"value": "valid"}))
		require.Error(t, result.schema.Validate(map[string]any{"value": ""}))
		if previous, ok := seen[result.id]; ok {
			require.Same(t, previous, result.schema)
		} else {
			seen[result.id] = result.schema
		}
	}
	for id, compiled := range seen {
		cached, err := CompileSchema(id)
		require.NoError(t, err)
		require.Same(t, compiled, cached)
	}
	// Failed compilation must release the lock and must not poison the cache.
	_, err := CompileSchema(base + "shared#/$defs/missing")
	require.Error(t, err)
	_, err = CompileSchema(base + "shared#/$defs/value")
	require.NoError(t, err)
}
