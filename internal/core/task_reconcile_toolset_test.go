package core

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/stretchr/testify/require"
)

// toolSetReconcileTest runs reconciliation against a real database and core
// service. The asynq client is a mock because tests invoke the pass directly.
type toolSetReconcileTest struct {
	ctx      context.Context
	db       database.DB
	service  *service
	enqueued *[]*asynq.Task

	// onEnqueue, when set, runs as each task is enqueued. It stands in for a
	// worker that picks a task up immediately, before the caller continues.
	onEnqueue func(ctx context.Context, task *asynq.Task)
}

// newToolSetReconcileTest creates the namespaces the scenarios select across.
// root.team_a and root.teamXa differ only where LIKE's `_` wildcard would match.
func newToolSetReconcileTest(t *testing.T) *toolSetReconcileTest {
	t.Helper()
	cfg := config.FromRoot(&sconfig.Root{})
	cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, rds := apredis.MustApplyTestConfig(cfg)
	h := httpf.CreateFactory(cfg, rds, nil, aplog.NewNoopLogger())
	cfg, e := encrypt.NewTestEncryptService(cfg, db)

	// Record every enqueued task so trigger tests can assert on them.
	r := &toolSetReconcileTest{ctx: context.Background(), db: db, enqueued: &[]*asynq.Task{}}
	ac := mock.NewMockClient(gomock.NewController(t))
	ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
			*r.enqueued = append(*r.enqueued, task)
			if r.onEnqueue != nil {
				r.onEnqueue(ctx, task)
			}
			return nil, nil
		}).AnyTimes()

	r.service = NewCoreService(cfg, db, e, rds, h, ac, aplog.NewNoopLogger()).(*service)

	for _, path := range []string{"root.team_a", "root.team_a.sub", "root.teamXa"} {
		require.NoError(t, db.CreateNamespace(r.ctx, &database.Namespace{Path: path}))
	}

	return r
}

// connection stores a connection with the supplied placement, state, and labels.
func (r *toolSetReconcileTest) connection(
	t *testing.T,
	namespace string,
	name string,
	state database.ConnectionState,
	labels map[string]string,
) *database.Connection {
	t.Helper()
	connection := &database.Connection{
		Id:                  apid.New(apid.PrefixConnection),
		Namespace:           namespace,
		Name:                scommon.ResourceName(name),
		State:               state,
		ConnectorId:         apid.New(apid.PrefixConnector),
		ConnectorGeneration: 1,
		Labels:              labels,
	}
	require.NoError(t, r.db.CreateConnection(r.ctx, connection))

	// Reload so the test sees the labels the reconciler reads.
	stored, err := r.db.GetConnection(r.ctx, connection.Id)
	require.NoError(t, err)
	return stored
}

// toolTemplate builds a compilable explicit template with the supplied key.
func toolTemplate(key string) toolsetschema.ToolTemplate {
	return toolsetschema.ToolTemplate{
		Key: key,
		Metadata: &toolsetschema.ToolTemplateMetadata{
			Name:        scommon.ResourceName(key),
			Labels:      map[string]string{"capability": key},
			Annotations: map[string]string{"owner": "reconcile-test"},
		},
		Spec: toolschema.ToolDefinition{
			Description: "Generated " + key,
			Verbs:       []string{"tool:" + key},
			InputSchema: []byte(`{"type":"object"}`),
			ProxyHTTP:   &toolschema.ProxyHTTP{Method: "GET", URL: "https://example.com/" + key},
		},
	}
}

// toolSet creates a ToolSet in root.team_a selecting provider=calendar across
// that namespace's subtree, with the requested first release state.
func (r *toolSetReconcileTest) toolSet(
	t *testing.T,
	name string,
	state toolsetschema.ToolSetReleaseState,
	templates ...toolsetschema.ToolTemplate,
) apid.ID {
	t.Helper()
	created, err := r.service.CreateToolSet(r.ctx, &toolsetschema.ToolSet{
		TypeMeta: meta.NewTypeMeta(toolsetschema.ToolSetKind),
		Metadata: meta.ObjectMeta{
			Namespace: "root.team_a",
			Name:      scommon.ResourceName(name),
		},
		Spec: toolsetschema.ToolSetSpec{
			ConnectionSelector: &toolsetschema.ConnectionSelector{
				MatchLabels: map[string]string{"provider": "calendar"},
			},
			Release: toolsetschema.ToolSetReleaseSpec{DesiredState: state},
			Definition: toolsetschema.ToolSetDefinition{
				Source: toolsetschema.ToolSetSource{
					Explicit: &toolsetschema.ExplicitSource{Tools: templates},
				},
			},
		},
	})
	require.NoError(t, err)
	return created.GetId()
}

// releasePatch requests the supplied state, optionally replacing the definition.
func releasePatch(
	state toolsetschema.ToolSetReleaseState,
	definition *toolsetschema.ToolSetDefinition,
) *toolsetschema.ToolSetPatch {
	patch := toolsetschema.NewToolSetPatch()
	patch.Spec.Release = &toolsetschema.ToolSetReleaseSpecPatch{DesiredState: &state}
	patch.Spec.Definition = definition
	return patch
}

// selectorPatch replaces the logical ToolSet's connection selector.
func selectorPatch(selector toolsetschema.ConnectionSelector) *toolsetschema.ToolSetPatch {
	patch := toolsetschema.NewToolSetPatch()
	patch.Spec.ConnectionSelector = &selector
	return patch
}

// generatedTools lists the live Tools one binding owns, sorted by source key.
func (r *toolSetReconcileTest) generatedTools(t *testing.T, binding *database.ToolSetBinding) []database.Tool {
	t.Helper()
	page := r.db.ListToolsBuilder().
		ForNamespaceMatcher("root.**").
		Limit(100).
		FetchPage(r.ctx)
	require.NoError(t, page.Error)

	owned := []database.Tool{}
	for _, tool := range page.Results {
		if tool.BindingId == binding.Id {
			owned = append(owned, tool)
		}
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].SourceKey < owned[j].SourceKey })
	return owned
}

// TestReconcileToolSetInstallsMatchingConnections covers first installation and
// which connections the selector picks.
func TestReconcileToolSetInstallsMatchingConnections(t *testing.T) {
	r := newToolSetReconcileTest(t)
	calendar := map[string]string{"provider": "calendar"}

	// One connection each for owner namespace, descendant, setup, other label,
	// and a namespace that only LIKE's `_` wildcard would select.
	owner := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, calendar)
	descendant := r.connection(t, "root.team_a.sub", "descendant", database.ConnectionStateConfigured, calendar)
	setup := r.connection(t, "root.team_a", "setup", database.ConnectionStateSetup, calendar)
	otherLabel := r.connection(t, "root.team_a", "other-label", database.ConnectionStateConfigured, map[string]string{"provider": "crm"})
	wildcard := r.connection(t, "root.teamXa", "wildcard", database.ConnectionStateConfigured, calendar)
	gone := r.connection(t, "root.team_a", "gone", database.ConnectionStateDisconnected, calendar)

	id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"), toolTemplate("create"))
	require.NoError(t, r.service.reconcileToolSet(r.ctx, id))

	// Both configured matches install the primary generation's complete inventory.
	for _, connection := range []*database.Connection{owner, descendant} {
		binding, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Equal(t, uint64(1), *binding.AppliedGeneration)

		tools := r.generatedTools(t, binding)
		require.Len(t, tools, 2)
		require.Equal(t, "create", tools[0].SourceKey)
		require.Equal(t, "list", tools[1].SourceKey)

		// A generated Tool lives beside its connection and projects its template.
		require.Equal(t, connection.Namespace, tools[1].Namespace)
		require.Equal(t, connection.Id, tools[1].ConnectionId)
		require.Equal(t, "Generated list", tools[1].Description)
		require.Equal(t, []string{"tool:list"}, tools[1].Verbs)
		require.Equal(t, "list", tools[1].Labels["capability"])
		require.Equal(t, "reconcile-test", tools[1].Annotations["owner"])
		require.Equal(t, connection.Id.String(), tools[1].Labels["apxy/cxn/-/id"])
		require.True(t, strings.HasPrefix(string(tools[1].Name), "calendar-tools-list-"))

		// The stored definition is the template's, readable as a normal Tool.
		hydrated, err := r.service.GetTool(r.ctx, tools[1].Id)
		require.NoError(t, err)
		require.Equal(t, "https://example.com/list", hydrated.GetResource().Spec.ProxyHTTP.URL)
		require.Equal(t, id, hydrated.GetManagedByToolSetId())
	}

	// An unconfigured connection is recorded as waiting, with nothing installed.
	pending, err := r.db.GetToolSetBindingForConnection(r.ctx, id, setup.Id)
	require.NoError(t, err)
	require.Nil(t, pending.AppliedGeneration)
	require.Empty(t, r.generatedTools(t, pending))

	// Nonmatching connections gain no binding, nor does a disconnected one.
	for _, connection := range []*database.Connection{otherLabel, wildcard, gone} {
		_, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.ErrorIs(t, err, database.ErrNotFound)
	}
}

// TestReconcileToolSetIsIdempotent shows a repeated pass changes nothing, so a
// task retry after a partial failure is safe.
func TestReconcileToolSetIsIdempotent(t *testing.T) {
	r := newToolSetReconcileTest(t)
	connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, map[string]string{"provider": "calendar"})
	id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))

	require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
	first, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
	require.NoError(t, err)
	before := r.generatedTools(t, first)

	require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
	second, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
	require.NoError(t, err)

	// The same binding keeps its counters and its Tool identities.
	require.Equal(t, first.Id, second.Id)
	require.Equal(t, first.Epoch, second.Epoch)
	require.Equal(t, first.InventoryRevision, second.InventoryRevision)
	require.Equal(t, before[0].Id, r.generatedTools(t, second)[0].Id)
}

// TestReconcileToolSetPublication covers the lifecycle around primary changes.
func TestReconcileToolSetPublication(t *testing.T) {
	calendar := map[string]string{"provider": "calendar"}

	t.Run("a draft-only toolset waits, then installs once published", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, calendar)
		id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStateDraft, toolTemplate("list"))

		// With no primary the binding records membership and stays pending.
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
		pending, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Nil(t, pending.AppliedGeneration)

		// Publishing lets the same pending binding install.
		_, err = r.service.UpdateToolSetGeneration(r.ctx, id, 1, releasePatch(toolsetschema.ToolSetReleaseStatePrimary, nil))
		require.NoError(t, err)
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))

		installed, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Equal(t, pending.Id, installed.Id)
		require.Equal(t, uint64(1), *installed.AppliedGeneration)
	})

	t.Run("a new primary leaves pins and reaches only new connections", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		existing := r.connection(t, "root.team_a", "existing", database.ConnectionStateConfigured, calendar)
		id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))

		// Publish generation 2 with a different inventory.
		definition := toolsetschema.ToolSetDefinition{Source: toolsetschema.ToolSetSource{
			Explicit: &toolsetschema.ExplicitSource{Tools: []toolsetschema.ToolTemplate{toolTemplate("search")}},
		}}
		_, err := r.service.CreateToolSetGeneration(r.ctx, id, releasePatch(toolsetschema.ToolSetReleaseStatePrimary, &definition))
		require.NoError(t, err)

		added := r.connection(t, "root.team_a", "added", database.ConnectionStateConfigured, calendar)
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))

		// The existing binding keeps generation 1 and its inventory.
		pinned, err := r.db.GetToolSetBindingForConnection(r.ctx, id, existing.Id)
		require.NoError(t, err)
		require.Equal(t, uint64(1), *pinned.AppliedGeneration)
		require.Equal(t, "list", r.generatedTools(t, pinned)[0].SourceKey)

		// The new connection installs the current primary.
		fresh, err := r.db.GetToolSetBindingForConnection(r.ctx, id, added.Id)
		require.NoError(t, err)
		require.Equal(t, uint64(2), *fresh.AppliedGeneration)
		require.Equal(t, "search", r.generatedTools(t, fresh)[0].SourceKey)
	})
}

// TestReconcileToolSetRemoval shows narrowing a selector removes only what the
// ToolSet owned on connections that stopped matching.
func TestReconcileToolSetRemoval(t *testing.T) {
	r := newToolSetReconcileTest(t)
	calendar := map[string]string{"provider": "calendar"}
	kept := r.connection(t, "root.team_a.sub", "kept", database.ConnectionStateConfigured, calendar)
	removed := r.connection(t, "root.team_a", "removed", database.ConnectionStateConfigured, calendar)

	// A second ToolSet and a standalone Tool share the connection losing its match.
	id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	other := r.toolSet(t, "other-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
	require.NoError(t, r.service.reconcileToolSet(r.ctx, other))

	standalone, err := r.service.CreateTool(r.ctx, &toolschema.Tool{
		TypeMeta: meta.NewTypeMeta(toolschema.ToolKind),
		Metadata: meta.ObjectMeta{Namespace: "root.team_a", Name: "standalone"},
		Spec: toolschema.ToolSpec{
			ConnectionRef: meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       "Connection",
				ID:         removed.Id.String(),
			},
			ToolDefinition: toolTemplate("standalone").Spec,
		},
	})
	require.NoError(t, err)

	staleBinding, err := r.db.GetToolSetBindingForConnection(r.ctx, id, removed.Id)
	require.NoError(t, err)
	require.Len(t, r.generatedTools(t, staleBinding), 1)

	// Narrow the selector to the descendant namespace and reconcile.
	_, err = r.service.UpdateToolSet(r.ctx, id, selectorPatch(toolsetschema.ConnectionSelector{
		Namespace:   "root.team_a.sub.**",
		MatchLabels: calendar,
	}))
	require.NoError(t, err)
	require.NoError(t, r.service.reconcileToolSet(r.ctx, id))

	// The nonmatching binding and its generated Tools are gone.
	_, err = r.db.GetToolSetBindingForConnection(r.ctx, id, removed.Id)
	require.ErrorIs(t, err, database.ErrNotFound)
	require.Empty(t, r.generatedTools(t, staleBinding))

	// The matching binding, the other ToolSet, and the standalone Tool are untouched.
	_, err = r.db.GetToolSetBindingForConnection(r.ctx, id, kept.Id)
	require.NoError(t, err)
	_, err = r.db.GetToolSetBindingForConnection(r.ctx, other, removed.Id)
	require.NoError(t, err)
	_, err = r.service.GetTool(r.ctx, standalone.GetId())
	require.NoError(t, err)
}

// TestReconcileToolSetMultipleToolSets attaches two ToolSets to one connection
// and checks that each owns an independent binding and inventory.
func TestReconcileToolSetMultipleToolSets(t *testing.T) {
	r := newToolSetReconcileTest(t)
	connection := r.connection(t, "root.team_a", "owner", database.ConnectionStateConfigured, map[string]string{"provider": "calendar"})

	// Matching template keys in two ToolSets are separate identities.
	first := r.toolSet(t, "first-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	second := r.toolSet(t, "second-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	require.NoError(t, r.service.reconcileToolSet(r.ctx, first))
	require.NoError(t, r.service.reconcileToolSet(r.ctx, second))

	firstBinding, err := r.db.GetToolSetBindingForConnection(r.ctx, first, connection.Id)
	require.NoError(t, err)
	secondBinding, err := r.db.GetToolSetBindingForConnection(r.ctx, second, connection.Id)
	require.NoError(t, err)

	require.NotEqual(t, firstBinding.Id, secondBinding.Id)
	require.NotEqual(t, r.generatedTools(t, firstBinding)[0].Id, r.generatedTools(t, secondBinding)[0].Id)
}

// TestToolSetChangesEnqueueReconcile checks which management writes request a pass.
func TestToolSetChangesEnqueueReconcile(t *testing.T) {
	r := newToolSetReconcileTest(t)

	// Creating a ToolSet requests a pass even for a draft.
	id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStateDraft, toolTemplate("list"))
	require.Len(t, *r.enqueued, 1)
	require.Equal(t, taskTypeReconcileToolSet, (*r.enqueued)[0].Type())
	require.Contains(t, string((*r.enqueued)[0].Payload()), id.String())

	// Metadata changes do not affect membership.
	annotations := map[string]string{"note": "changed"}
	metadataPatch := toolsetschema.NewToolSetPatch()
	metadataPatch.Metadata.Annotations = &annotations
	_, err := r.service.UpdateToolSet(r.ctx, id, metadataPatch)
	require.NoError(t, err)
	require.Len(t, *r.enqueued, 1)

	// Selector changes and publication each request another pass.
	_, err = r.service.UpdateToolSet(r.ctx, id, selectorPatch(toolsetschema.ConnectionSelector{
		Namespace:   "root.team_a.**",
		MatchLabels: map[string]string{"provider": "crm"},
	}))
	require.NoError(t, err)
	require.Len(t, *r.enqueued, 2)

	_, err = r.service.UpdateToolSetGeneration(r.ctx, id, 1, releasePatch(toolsetschema.ToolSetReleaseStatePrimary, nil))
	require.NoError(t, err)
	require.Len(t, *r.enqueued, 3)
}

// TestUpdateToolSetCombinedPatchReconcile covers one patch changing the selector
// and publishing a generation, which commit as separate writes.
func TestUpdateToolSetCombinedPatchReconcile(t *testing.T) {
	calendar := map[string]string{"provider": "calendar"}
	crm := map[string]string{"provider": "crm"}

	// combinedPatch selects crm connections and publishes a search-only generation.
	combinedPatch := func() *toolsetschema.ToolSetPatch {
		patch := releasePatch(toolsetschema.ToolSetReleaseStatePrimary, &toolsetschema.ToolSetDefinition{
			Source: toolsetschema.ToolSetSource{
				Explicit: &toolsetschema.ExplicitSource{Tools: []toolsetschema.ToolTemplate{toolTemplate("search")}},
			},
		})
		patch.Spec.ConnectionSelector = &toolsetschema.ConnectionSelector{MatchLabels: crm}
		return patch
	}

	t.Run("a worker starting at any enqueue installs the new primary", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))

		// The connection only matches once the patch's selector applies.
		connection := r.connection(t, "root.team_a", "crm", database.ConnectionStateConfigured, crm)

		// Finish a pass inside every enqueue, as the fastest possible worker would.
		passes := 0
		r.onEnqueue = func(ctx context.Context, _ *asynq.Task) {
			passes++
			require.NoError(t, r.service.reconcileToolSet(ctx, id))
		}
		updated, err := r.service.UpdateToolSet(r.ctx, id, combinedPatch())
		require.NoError(t, err)
		require.Equal(t, uint64(2), updated.GetGeneration())
		require.Positive(t, passes)

		// No pass ran early enough to pin the connection to generation 1.
		binding, err := r.db.GetToolSetBindingForConnection(r.ctx, id, connection.Id)
		require.NoError(t, err)
		require.Equal(t, uint64(2), *binding.AppliedGeneration)
		require.Equal(t, "search", r.generatedTools(t, binding)[0].SourceKey)
	})

	t.Run("a committed selector still reconciles when publication fails", func(t *testing.T) {
		r := newToolSetReconcileTest(t)
		id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
		matching := r.connection(t, "root.team_a", "calendar", database.ConnectionStateConfigured, calendar)
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
		dropped, err := r.db.GetToolSetBindingForConnection(r.ctx, id, matching.Id)
		require.NoError(t, err)

		// Cancel the request right after the selector commits, so everything the
		// patch does next fails and the enqueue has only that canceled context.
		ctx, cancel := context.WithCancel(r.ctx)
		defer cancel()
		r.service.db = &cancelAfterSelectorDB{DB: r.db, cancel: cancel}
		var enqueueErr error
		r.onEnqueue = func(ctx context.Context, _ *asynq.Task) { enqueueErr = ctx.Err() }
		*r.enqueued = nil

		_, err = r.service.UpdateToolSet(ctx, id, combinedPatch())
		require.ErrorIs(t, err, context.Canceled)

		// The update failed, but its selector stands and one pass was still requested
		// on a context that can reach the queue.
		stored, err := r.db.GetToolSet(r.ctx, id)
		require.NoError(t, err)
		require.Equal(t, crm, stored.ConnectionSelector.MatchLabels)
		require.Len(t, *r.enqueued, 1)
		require.NoError(t, enqueueErr)

		// That pass applies the committed selector and removes the stale binding,
		// which leaves generation 1 as the only one published.
		r.service.db = r.db
		require.NoError(t, r.service.reconcileToolSet(r.ctx, id))
		_, err = r.db.GetToolSetBindingForConnection(r.ctx, id, matching.Id)
		require.ErrorIs(t, err, database.ErrNotFound)
		require.Empty(t, r.generatedTools(t, dropped))
	})
}

// cancelAfterSelectorDB cancels the request context once a selector update has
// committed, simulating a client that goes away partway through a combined patch.
type cancelAfterSelectorDB struct {
	database.DB
	cancel context.CancelFunc
}

// UpdateToolSetConnectionSelector commits through the real database, then cancels.
func (d *cancelAfterSelectorDB) UpdateToolSetConnectionSelector(
	ctx context.Context,
	id apid.ID,
	expectedRevision uint64,
	selector toolsetschema.ConnectionSelector,
) (*database.ToolSet, error) {
	logical, err := d.DB.UpdateToolSetConnectionSelector(ctx, id, expectedRevision, selector)
	d.cancel()
	return logical, err
}

// TestReconcileToolSetTask covers payload handling in the asynq handler.
func TestReconcileToolSetTask(t *testing.T) {
	r := newToolSetReconcileTest(t)

	// Undecodable or empty payloads can never succeed, so they skip retries.
	err := r.service.reconcileToolSetTask(r.ctx, asynq.NewTask(taskTypeReconcileToolSet, []byte("{")))
	require.True(t, errors.Is(err, asynq.SkipRetry))
	err = r.service.reconcileToolSetTask(r.ctx, asynq.NewTask(taskTypeReconcileToolSet, []byte(`{}`)))
	require.True(t, errors.Is(err, asynq.SkipRetry))

	// A ToolSet deleted before the task ran is a successful no-op.
	id := r.toolSet(t, "calendar-tools", toolsetschema.ToolSetReleaseStatePrimary, toolTemplate("list"))
	require.NoError(t, r.db.DeleteToolSet(r.ctx, id))
	task, err := newReconcileToolSetTask(id)
	require.NoError(t, err)
	require.NoError(t, r.service.reconcileToolSetTask(r.ctx, task))
}

// TestGeneratedToolName checks generated names stay valid, stable, and distinct.
func TestGeneratedToolName(t *testing.T) {
	toolSetId := apid.New(apid.PrefixToolSet)
	connectionId := apid.New(apid.PrefixConnection)
	long := scommon.ResourceName(strings.Repeat("a", 40) + "-" + strings.Repeat("b", 22))

	name := generatedToolName("calendar-tools", "list", toolSetId, connectionId, "list")
	require.NoError(t, name.Validate())
	require.True(t, strings.HasPrefix(string(name), "calendar-tools-list-"))

	// The same identity always produces the same name.
	require.Equal(t, name, generatedToolName("calendar-tools", "list", toolSetId, connectionId, "list"))

	// A template with no name falls back to the ToolSet stem.
	require.True(t, strings.HasPrefix(string(generatedToolName("calendar-tools", "", toolSetId, connectionId, "list")), "calendar-tools-"))

	// Maximum-length inputs are truncated to a valid name, including when the cut
	// lands on a separator.
	require.NoError(t, generatedToolName(long, long, toolSetId, connectionId, "list").Validate())

	// Each part of the logical identity distinguishes the name.
	require.NotEqual(t, name, generatedToolName("calendar-tools", "list", toolSetId, connectionId, "other"))
	require.NotEqual(t, name, generatedToolName("calendar-tools", "list", toolSetId, apid.New(apid.PrefixConnection), "list"))
	require.NotEqual(t, name, generatedToolName("calendar-tools", "list", apid.New(apid.PrefixToolSet), connectionId, "list"))
}
