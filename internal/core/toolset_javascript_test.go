package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// javascriptToolTemplate keeps the normal generated metadata while selecting a
// JavaScript executor whose helpers are resolved separately for each connection.
func javascriptToolTemplate(key, source string) toolsetschema.ToolTemplate {
	template := toolTemplate(key)
	template.Spec.ProxyHTTP = nil
	template.Spec.Javascript = &source
	return template
}

// toolSetJavascriptConnector stores a published helper generation without
// migrating the connection that refers to this logical connector.
func toolSetJavascriptConnector(t *testing.T, r *toolSetReconcileTest, connection *database.Connection, generation uint64, source string) {
	t.Helper()
	definition, err := json.Marshal(map[string]any{
		"auth": map[string]string{"type": "no-auth"}, "javascript": source,
	})
	require.NoError(t, err)
	encrypted, err := r.service.encrypt.EncryptForNamespace(r.ctx, connection.Namespace, definition)
	require.NoError(t, err)
	require.NoError(t, r.db.UpsertConnectorGeneration(r.ctx, &database.ConnectorWithDefinition{
		Id: connection.ConnectorId, Namespace: connection.Namespace,
		Generation: generation, State: database.ConnectorGenerationStatePrimary,
		EncryptedDefinition: encrypted,
	}))
}

// TestToolSetJavaScriptReconciliation validates each selected connection's own
// helpers and never activates part of a mixed inventory after a failed template.
func TestToolSetJavaScriptReconciliation(t *testing.T) {
	r := newToolSetReconcileTest(t)
	var logs bytes.Buffer
	r.service.logger = slog.New(slog.NewTextHandler(&logs, nil))
	labels := map[string]string{"provider": "calendar"}
	good := r.connection(t, "root.team_a", "good", database.ConnectionStateConfigured, labels)
	bad := r.connection(t, "root.team_a.sub", "bad", database.ConnectionStateConfigured, labels)
	toolSetJavascriptConnector(t, r, good, 1, `function selected() { return "supported"; }`)
	toolSetJavascriptConnector(t, r, good, 2, `throw "new primary must not initialize";`)
	toolSetJavascriptConnector(t, r, bad, 1, `function selected() { return "unsupported"; }`)
	const source = `
		if (selected() !== "supported") throw "private helper incompatibility";
		async function execute(params, context) {
			const response = await context.connections[0].fetch(cfg.apiBase + "/customers/" + params.id);
			return response.json();
		}
	`

	// Generation compilation has no selected library; initialization waits for installation.
	id := r.toolSet(t, "javascript-tools", toolsetschema.ToolSetReleaseStatePrimary,
		toolTemplate("http-first"), javascriptToolTemplate("javascript", source))
	task, err := newReconcileToolSetTask(id)
	require.NoError(t, err)
	require.NoError(t, r.service.reconcileToolSetTask(r.ctx, task))
	require.Contains(t, logs.String(), "toolset binding remains pending after template validation")
	require.NotContains(t, logs.String(), "private helper incompatibility")

	// A compatible connection installs both templates despite another connection's failure.
	installed, err := r.db.GetToolSetBindingForConnection(r.ctx, id, good.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(1), *installed.AppliedGeneration)
	generated := r.generatedTools(t, installed)
	require.Len(t, generated, 2)
	jsTool, err := r.service.GetTool(r.ctx, generated[1].Id)
	require.NoError(t, err)
	require.Equal(t, source, *jsTool.GetResource().Spec.Javascript)
	require.Nil(t, jsTool.GetResource().Spec.ProxyHTTP)
	require.Equal(t, good.Id, jsTool.GetConnectionId())

	// Even the earlier HTTP template remains absent from the incompatible binding.
	pending, err := r.db.GetToolSetBindingForConnection(r.ctx, id, bad.Id)
	require.NoError(t, err)
	require.Nil(t, pending.AppliedGeneration)
	require.Zero(t, pending.InventoryRevision)
	require.Empty(t, r.generatedTools(t, pending))

	// A connector migration supplies compatible helpers; its follow-up queues a fresh pass.
	toolSetJavascriptConnector(t, r, bad, 2, `function selected() { return "supported"; }`)
	_, err = r.db.UpdateConnectionForGenerationMigration(r.ctx, database.ConnectionGenerationMigrationUpdate{
		Id: bad.Id, ConnectorId: bad.ConnectorId, ConnectorGeneration: 2, UserLabels: labels,
	})
	require.NoError(t, err)
	*r.enqueued = nil
	require.NoError(t, r.service.reconcileToolSetsAfterConnectionMigrationV1(r.ctx, bad.Id, 2))
	require.Len(t, *r.enqueued, 1)
	require.Equal(t, taskTypeReconcileToolSet, (*r.enqueued)[0].Type())

	// Processing that queued task installs the same pending binding and preserves installed pins.
	require.NoError(t, r.service.reconcileToolSetTask(r.ctx, (*r.enqueued)[0]))
	retried, err := r.db.GetToolSetBindingForConnection(r.ctx, id, bad.Id)
	require.NoError(t, err)
	require.Equal(t, pending.Id, retried.Id)
	require.Equal(t, uint64(1), *retried.AppliedGeneration)
	require.Len(t, r.generatedTools(t, retried), 2)
	unchanged, err := r.db.GetToolSetBindingForConnection(r.ctx, id, good.Id)
	require.NoError(t, err)
	require.Equal(t, installed.Epoch, unchanged.Epoch)
	require.Equal(t, installed.InventoryRevision, unchanged.InventoryRevision)
	require.Equal(t, generated[1].Id, r.generatedTools(t, unchanged)[1].Id)

	// A new ToolSet primary affects new connections without migrating installed pins.
	definition := toolsetschema.ToolSetDefinition{Source: toolsetschema.ToolSetSource{
		Explicit: &toolsetschema.ExplicitSource{Tools: []toolsetschema.ToolTemplate{
			javascriptToolTemplate("replacement", source),
		}},
	}}
	_, err = r.service.CreateToolSetGeneration(r.ctx, id, releasePatch(toolsetschema.ToolSetReleaseStatePrimary, &definition))
	require.NoError(t, err)
	added := r.connection(t, "root.team_a", "added", database.ConnectionStateConfigured, labels)
	toolSetJavascriptConnector(t, r, added, 1, `function selected() { return "supported"; }`)
	require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
	fresh, err := r.db.GetToolSetBindingForConnection(r.ctx, id, added.Id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), *fresh.AppliedGeneration)
	require.Equal(t, "replacement", r.generatedTools(t, fresh)[0].SourceKey)
	for _, connection := range []*database.Connection{good, bad} {
		pinned, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Equal(t, uint64(1), *pinned.AppliedGeneration)
		require.Len(t, r.generatedTools(t, pinned), 2)
	}
}

// TestToolSetJavaScriptPlanOwnsValidationDefinition keeps the code checked at
// installation identical to the serialized definition captured by the plan.
func TestToolSetJavaScriptPlanOwnsValidationDefinition(t *testing.T) {
	r := newToolSetReconcileTest(t)
	connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, map[string]string{"provider": "calendar"})
	toolSetJavascriptConnector(t, r, connection, 1, `function selected() { return true; }`)
	const source = `if (!selected()) throw "invalid helper"; async function execute() { return cfg.tenant; }`
	template := javascriptToolTemplate("lookup", source)
	template.Spec.Limits = &toolschema.ExecutionLimits{MaxStackDepth: util.ToPtr(256)}
	definition := toolsetschema.ToolSetDefinition{Source: toolsetschema.ToolSetSource{
		Explicit: &toolsetschema.ExplicitSource{Tools: []toolsetschema.ToolTemplate{template}},
	}}
	id := r.toolSet(t, "owned-plan", toolsetschema.ToolSetReleaseStatePrimary, template)
	logical, err := r.db.GetToolSet(r.ctx, id)
	require.NoError(t, err)
	binding, err := r.db.CreateToolSetBinding(r.ctx, id, connection.Id, logical.SelectorRevision)
	require.NoError(t, err)
	plan, err := newToolSetInstallPlan(1, &definition)
	require.NoError(t, err)

	// Mutating all authored buffers after compilation must not replace the checked source or policy.
	*template.Spec.Javascript = `throw "changed source"; async function execute() {}`
	template.Spec.InputSchema[0] = '!'
	*template.Spec.Limits.MaxStackDepth = 0
	install, err := r.service.inventoryFor(r.ctx, plan, logical, binding, connection)
	require.NoError(t, err)
	require.NotNil(t, install.ExpectedConnector)
	require.Equal(t, database.ConnectorGenerationId{Id: connection.ConnectorId, Generation: 1}, *install.ExpectedConnector)
	require.Len(t, install.Tools, 1)
	plaintext, err := r.service.encrypt.Decrypt(r.ctx, install.Tools[0].EncryptedDefinition)
	require.NoError(t, err)
	var captured toolschema.ToolDefinition
	require.NoError(t, json.Unmarshal(plaintext, &captured))
	require.Equal(t, source, *captured.Javascript)
	require.JSONEq(t, `{"type":"object"}`, string(captured.InputSchema))
	require.Equal(t, 256, *captured.Limits.MaxStackDepth)
}

// TestToolSetJavaScriptAllowsEditedHelpers documents that preparation does not
// prevent a draft helper edit within the same connector generation from installing.
func TestToolSetJavaScriptAllowsEditedHelpers(t *testing.T) {
	r := newToolSetReconcileTest(t)
	connection := r.connection(t, "root.team_a", "draft-owner", database.ConnectionStateConfigured, map[string]string{"provider": "calendar"})
	encrypted, err := r.service.encrypt.EncryptStringForNamespace(r.ctx, connection.Namespace,
		`{"auth":{"type":"no-auth"},"javascript":"function selected() { return true; }"}`)
	require.NoError(t, err)
	connector := &database.ConnectorWithDefinition{
		Id: connection.ConnectorId, Namespace: connection.Namespace,
		Generation: 1, State: database.ConnectorGenerationStateDraft, EncryptedDefinition: encrypted,
	}
	require.NoError(t, r.db.UpsertConnectorGeneration(r.ctx, connector))
	template := javascriptToolTemplate("lookup", `if (!selected()) throw "bad helper"; async function execute() { return true; }`)
	id := r.toolSet(t, "draft-helper-tools", toolsetschema.ToolSetReleaseStatePrimary,
		template)
	logical, err := r.db.GetToolSet(r.ctx, id)
	require.NoError(t, err)
	pending, err := r.db.CreateToolSetBinding(r.ctx, id, connection.Id, logical.SelectorRevision)
	require.NoError(t, err)
	plan, err := newToolSetInstallPlan(1, &toolsetschema.ToolSetDefinition{Source: toolsetschema.ToolSetSource{
		Explicit: &toolsetschema.ExplicitSource{Tools: []toolsetschema.ToolTemplate{template}},
	}})
	require.NoError(t, err)

	// Validate against the draft, then change its helpers without changing its generation.
	install, err := r.service.inventoryFor(r.ctx, plan, logical, pending, connection)
	require.NoError(t, err)
	connector.EncryptedDefinition, err = r.service.encrypt.EncryptStringForNamespace(r.ctx, connection.Namespace,
		`{"auth":{"type":"no-auth"},"javascript":"function selected() { return false; }"}`)
	require.NoError(t, err)
	require.NoError(t, r.db.UpsertConnectorGeneration(r.ctx, connector))
	_, err = r.db.InstallExplicitToolSetInventory(r.ctx, install)
	require.NoError(t, err)

	// Installation accepts the edit even though fresh validation would fail.
	_, err = r.service.inventoryFor(r.ctx, plan, logical, pending, connection)
	require.ErrorIs(t, err, ErrInvalidArgument)
	installed, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
	require.NoError(t, err)
	require.Equal(t, pending.Id, installed.Id)
	require.Equal(t, uint64(1), *installed.AppliedGeneration)
	require.Len(t, r.generatedTools(t, installed), 1)
}

// toolSetHelperReadFailureDB injects a dependency failure for one connection's
// helper lookup while letting the rest of a reconciliation pass use real storage.
type toolSetHelperReadFailureDB struct {
	database.DB
	connectorID apid.ID
	failure     error
}

// GetConnectorGeneration fails only the selected connector's helper read.
func (d *toolSetHelperReadFailureDB) GetConnectorGeneration(ctx context.Context, id apid.ID, generation uint64) (*database.ConnectorWithDefinition, error) {
	if id == d.connectorID {
		return nil, d.failure
	}
	return d.DB.GetConnectorGeneration(ctx, id, generation)
}

// TestToolSetJavaScriptReconciliationRetriesDependencies keeps retryable failures
// visible even when the same pass encounters permanently incompatible helpers.
func TestToolSetJavaScriptReconciliationRetriesDependencies(t *testing.T) {
	for name, failure := range map[string]error{
		"unavailable": errors.New("helper storage unavailable"),
		"timeout":     context.DeadlineExceeded,
		"canceled":    context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			r := newToolSetReconcileTest(t)
			labels := map[string]string{"provider": "calendar"}
			good := r.connection(t, "root.team_a", "good", database.ConnectionStateConfigured, labels)
			bad := r.connection(t, "root.team_a", "bad", database.ConnectionStateConfigured, labels)
			delayed := r.connection(t, "root.team_a", "delayed", database.ConnectionStateConfigured, labels)
			toolSetJavascriptConnector(t, r, good, 1, `function selected() { return true; }`)
			toolSetJavascriptConnector(t, r, bad, 1, `function selected() { return false; }`)
			toolSetJavascriptConnector(t, r, delayed, 1, `function selected() { return true; }`)
			id := r.toolSet(t, "mixed-failures", toolsetschema.ToolSetReleaseStatePrimary,
				javascriptToolTemplate("lookup", `if (!selected()) throw "incompatible"; async function execute() {}`))
			task, err := newReconcileToolSetTask(id)
			require.NoError(t, err)
			r.service.db = &toolSetHelperReadFailureDB{DB: r.db, connectorID: delayed.ConnectorId, failure: failure}

			// The bad helper must not suppress retries needed by the unavailable dependency.
			err = r.service.reconcileToolSetTask(r.ctx, task)
			require.ErrorIs(t, err, failure)
			require.NotErrorIs(t, err, ErrInvalidArgument)
			require.NotErrorIs(t, err, asynq.SkipRetry)
			installed, err := r.db.GetToolSetBindingForConnection(r.ctx, id, good.Id)
			require.NoError(t, err)
			require.NotNil(t, installed.AppliedGeneration)
			for _, connection := range []*database.Connection{bad, delayed} {
				pending, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
				require.NoError(t, err)
				require.Nil(t, pending.AppliedGeneration)
				require.Empty(t, r.generatedTools(t, pending))
			}

			// Once storage recovers, the retry installs the delayed binding and completes.
			r.service.db = r.db
			require.NoError(t, r.service.reconcileToolSetTask(r.ctx, task))
			recovered, err := r.db.GetToolSetBindingForConnection(r.ctx, id, delayed.Id)
			require.NoError(t, err)
			require.NotNil(t, recovered.AppliedGeneration)
			pending, err := r.db.GetToolSetBindingForConnection(r.ctx, id, bad.Id)
			require.NoError(t, err)
			require.Nil(t, pending.AppliedGeneration)
		})
	}
}

// TestToolSetJavaScriptAllowsSameGenerationHelperChanges accepts edits or key
// rotation between template validations when the connector generation is unchanged.
func TestToolSetJavaScriptAllowsSameGenerationHelperChanges(t *testing.T) {
	f := newToolAuthorizationFixture(t, nil)
	first := f.snapshot.Connector
	second := first
	second.EncryptedDefinition.Data = "changed-connector-definition"
	definition := toolsetschema.ToolSetDefinition{Source: toolsetschema.ToolSetSource{
		Explicit: &toolsetschema.ExplicitSource{Tools: []toolsetschema.ToolTemplate{
			javascriptToolTemplate("first", `if (selected() !== 1) throw "first helper"; async function execute() {}`),
			javascriptToolTemplate("second", `if (selected() !== 2) throw "second helper"; async function execute() {}`),
		}},
	}}
	plan, err := newToolSetInstallPlan(1, &definition)
	require.NoError(t, err)

	// Each template validates independently; changes within the generation are accepted.
	gomock.InOrder(
		f.db.EXPECT().GetConnectorGeneration(gomock.Any(), first.Id, first.Generation).Return(&first, nil),
		f.encrypt.EXPECT().DecryptString(gomock.Any(), first.EncryptedDefinition).
			Return(`{"javascript":"function selected() { return 1; }"}`, nil),
		f.encrypt.EXPECT().EncryptForNamespace(gomock.Any(), f.snapshot.Connection.Namespace, gomock.Any()).
			Return(f.snapshot.Revision.EncryptedDefinition, nil),
		f.db.EXPECT().GetConnectorGeneration(gomock.Any(), second.Id, second.Generation).Return(&second, nil),
		f.encrypt.EXPECT().DecryptString(gomock.Any(), second.EncryptedDefinition).
			Return(`{"javascript":"function selected() { return 2; }"}`, nil),
		f.encrypt.EXPECT().EncryptForNamespace(gomock.Any(), f.snapshot.Connection.Namespace, gomock.Any()).
			Return(f.snapshot.Revision.EncryptedDefinition, nil),
	)
	install, err := f.service.inventoryFor(context.Background(), plan,
		&database.ToolSet{Id: f.snapshot.ToolSetId, Name: "mixed-helpers", SelectorRevision: 1},
		&database.ToolSetBinding{Id: apid.New(apid.PrefixToolSetBinding), Epoch: 1}, &f.snapshot.Connection)

	// The candidate only carries connector identity, so both prepared templates are usable.
	require.NoError(t, err)
	require.Len(t, install.Tools, 2)
	require.Equal(t, &database.ConnectorGenerationId{Id: first.Id, Generation: first.Generation}, install.ExpectedConnector)
}

// TestToolSetJavaScriptCompilationDefersInitialization checks that publication
// admits syntax independently from any connection's helper-dependent startup.
func TestToolSetJavaScriptCompilationDefersInitialization(t *testing.T) {
	for name, source := range map[string]string{
		"helper initialization": `const ready = helper(); async function execute() { return ready; }`,
		"throw":                 `throw "private initialization"; async function execute() {}`,
		"loop":                  `while (true) {} async function execute() {}`,
	} {
		t.Run(name, func(t *testing.T) {
			// Source-only generation compilation cannot run an unavailable helper, throw, or loop.
			definition := javascriptToolTemplate("lookup", source).Spec
			require.NoError(t, compileToolDefinition(&definition, "explicit.tools[0].spec"))
		})
	}
	for name, source := range map[string]string{
		"syntax":     `async function execute( {`,
		"sync entry": `function execute() {}`,
		"host name":  `const fetch = 1; async function execute() {}`,
		"no execute": `const ready = true;`,
	} {
		t.Run(name, func(t *testing.T) {
			// Invalid static contracts fail before any connection-specific installation work.
			definition := javascriptToolTemplate("lookup", source).Spec
			err := compileToolDefinition(&definition, "explicit.tools[0].spec")
			require.ErrorIs(t, err, ErrInvalidArgument)
			require.Contains(t, err.Error(), "explicit.tools[0].spec")
		})
	}
}
