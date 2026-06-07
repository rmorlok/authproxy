package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/history"
	"github.com/cschleiden/go-workflows/backend/metrics"
	wfcore "github.com/cschleiden/go-workflows/core"
	"github.com/cschleiden/go-workflows/diag"
	wflib "github.com/cschleiden/go-workflows/workflow"
	"github.com/gin-gonic/gin"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestWorkflowMonitoringRoutes(t *testing.T) {
	type TestSetup struct {
		Gin     *gin.Engine
		Auth    *auth2.AuthTestUtil
		Backend *fakeWorkflowMonitoringBackend
	}

	setup := func(t *testing.T, cfg config.C) *TestSetup {
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		_, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)
		backend := &fakeWorkflowMonitoringBackend{}
		routes := NewWorkflowMonitoringRoutes(auth, backend, pagination.NewRandomCursorEncryptor())

		r := apgin.ForTest(nil)
		routes.Register(r)

		return &TestSetup{
			Gin:     r,
			Auth:    authUtil,
			Backend: backend,
		}
	}

	t.Run("list instances", func(t *testing.T) {
		t.Run("unauthorized", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/workflow-monitoring/instances", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.Auth.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/workflow-monitoring/instances",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			tu := setup(t, nil)
			createdAt := time.Now().UTC()
			tu.Backend.instances = []*diag.WorkflowInstanceRef{
				{
					Instance:  wfcore.NewWorkflowInstance("wf-a", "exec-a"),
					CreatedAt: createdAt,
					State:     wfcore.WorkflowInstanceStateActive,
					Queue:     "default",
				},
				{
					Instance:  wfcore.NewWorkflowInstance("wf-b", "exec-b"),
					CreatedAt: createdAt.Add(-time.Minute),
					State:     wfcore.WorkflowInstanceStateFinished,
					Queue:     "default",
				},
			}

			w := httptest.NewRecorder()
			req, err := tu.Auth.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/workflow-monitoring/instances?limit=1",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "workflow_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, 2, tu.Backend.listCount)

			var resp ListWorkflowInstancesResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Items, 1)
			require.Equal(t, "wf-a", resp.Items[0].Instance.InstanceID)
			require.NotEmpty(t, resp.Cursor)
		})
	})

	t.Run("get instance includes history", func(t *testing.T) {
		tu := setup(t, nil)
		instance := wfcore.NewWorkflowInstance("wf-a", "exec-a")
		tu.Backend.instance = &diag.WorkflowInstanceRef{
			Instance: instance,
			State:    wfcore.WorkflowInstanceStateFinished,
			Queue:    "default",
		}
		tu.Backend.history = []*history.Event{
			history.NewPendingEvent(time.Now(), history.EventType_WorkflowExecutionStarted, nil),
		}

		w := httptest.NewRecorder()
		req, err := tu.Auth.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/workflow-monitoring/instances/wf-a/exec-a",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "workflow_monitoring", "get"),
		)
		require.NoError(t, err)

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp WorkflowInstanceInfoJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, "wf-a", resp.Instance.InstanceID)
		require.Len(t, resp.History, 1)
		require.Equal(t, "WorkflowExecutionStarted", resp.History[0].Type)
	})

	t.Run("history not found", func(t *testing.T) {
		tu := setup(t, nil)
		tu.Backend.historyErr = wfbackend.ErrInstanceNotFound

		w := httptest.NewRecorder()
		req, err := tu.Auth.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/workflow-monitoring/instances/missing/exec/history",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "workflow_monitoring", "get"),
		)
		require.NoError(t, err)

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("tree success", func(t *testing.T) {
		tu := setup(t, nil)
		tu.Backend.tree = &diag.WorkflowInstanceTree{
			WorkflowInstanceRef: &diag.WorkflowInstanceRef{
				Instance: wfcore.NewWorkflowInstance("wf-a", "exec-a"),
				State:    wfcore.WorkflowInstanceStateActive,
			},
			WorkflowName: "core.connection.disconnect.v1",
		}

		w := httptest.NewRecorder()
		req, err := tu.Auth.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/workflow-monitoring/instances/wf-a/exec-a/tree",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "workflow_monitoring", "get"),
		)
		require.NoError(t, err)

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp WorkflowInstanceTreeJson
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, "core.connection.disconnect.v1", resp.WorkflowName)
	})

	t.Run("cancel requires manage", func(t *testing.T) {
		tu := setup(t, nil)

		w := httptest.NewRecorder()
		req, err := tu.Auth.NewSignedRequestForActorExternalId(
			http.MethodPost,
			"/workflow-monitoring/instances/wf-a/exec-a/_cancel",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "workflow_monitoring", "list"),
		)
		require.NoError(t, err)

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code)
		require.False(t, tu.Backend.cancelCalled)
	})

	t.Run("cancel success", func(t *testing.T) {
		tu := setup(t, nil)

		w := httptest.NewRecorder()
		req, err := tu.Auth.NewSignedRequestForActorExternalId(
			http.MethodPost,
			"/workflow-monitoring/instances/wf-a/exec-a/_cancel",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "workflow_monitoring", "manage"),
		)
		require.NoError(t, err)

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.True(t, tu.Backend.cancelCalled)
		require.Equal(t, "wf-a", tu.Backend.cancelInstance.InstanceID)
	})

	t.Run("remove unfinished maps to conflict", func(t *testing.T) {
		tu := setup(t, nil)
		tu.Backend.removeErr = wfbackend.ErrInstanceNotFinished

		w := httptest.NewRecorder()
		req, err := tu.Auth.NewSignedRequestForActorExternalId(
			http.MethodDelete,
			"/workflow-monitoring/instances/wf-a/exec-a",
			nil,
			"root",
			"some-actor",
			aschema.PermissionsSingle("root.**", "workflow_monitoring", "manage"),
		)
		require.NoError(t, err)

		tu.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusConflict, w.Code)
	})
}

type fakeWorkflowMonitoringBackend struct {
	instances []*diag.WorkflowInstanceRef
	instance  *diag.WorkflowInstanceRef
	history   []*history.Event
	tree      *diag.WorkflowInstanceTree

	listErr     error
	instanceErr error
	historyErr  error
	treeErr     error
	cancelErr   error
	removeErr   error

	listCount      int
	cancelCalled   bool
	cancelInstance *wfcore.WorkflowInstance
	removeCalled   bool
	removeInstance *wfcore.WorkflowInstance
}

func (f *fakeWorkflowMonitoringBackend) GetWorkflowInstances(ctx context.Context, afterInstanceID, afterExecutionID string, count int) ([]*diag.WorkflowInstanceRef, error) {
	f.listCount = count
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.instances, nil
}

func (f *fakeWorkflowMonitoringBackend) GetWorkflowInstance(ctx context.Context, instance *wfcore.WorkflowInstance) (*diag.WorkflowInstanceRef, error) {
	if f.instanceErr != nil {
		return nil, f.instanceErr
	}
	return f.instance, nil
}

func (f *fakeWorkflowMonitoringBackend) GetWorkflowTree(ctx context.Context, instance *wfcore.WorkflowInstance) (*diag.WorkflowInstanceTree, error) {
	if f.treeErr != nil {
		return nil, f.treeErr
	}
	return f.tree, nil
}

func (f *fakeWorkflowMonitoringBackend) CreateWorkflowInstance(ctx context.Context, instance *wflib.Instance, event *history.Event) error {
	return errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) CancelWorkflowInstance(ctx context.Context, instance *wflib.Instance, cancelEvent *history.Event) error {
	f.cancelCalled = true
	f.cancelInstance = instance
	return f.cancelErr
}

func (f *fakeWorkflowMonitoringBackend) RemoveWorkflowInstance(ctx context.Context, instance *wflib.Instance) error {
	f.removeCalled = true
	f.removeInstance = instance
	return f.removeErr
}

func (f *fakeWorkflowMonitoringBackend) RemoveWorkflowInstances(ctx context.Context, options ...wfbackend.RemovalOption) error {
	return errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) GetWorkflowInstanceState(ctx context.Context, instance *wflib.Instance) (wfcore.WorkflowInstanceState, error) {
	return wfcore.WorkflowInstanceStateActive, nil
}

func (f *fakeWorkflowMonitoringBackend) GetWorkflowInstanceHistory(ctx context.Context, instance *wflib.Instance, lastSequenceID *int64) ([]*history.Event, error) {
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.history, nil
}

func (f *fakeWorkflowMonitoringBackend) SignalWorkflow(ctx context.Context, instanceID string, event *history.Event) error {
	return errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) PrepareWorkflowQueues(ctx context.Context, queues []wflib.Queue) error {
	return nil
}

func (f *fakeWorkflowMonitoringBackend) PrepareActivityQueues(ctx context.Context, queues []wflib.Queue) error {
	return nil
}

func (f *fakeWorkflowMonitoringBackend) GetWorkflowTask(ctx context.Context, queues []wflib.Queue) (*wfbackend.WorkflowTask, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) ExtendWorkflowTask(ctx context.Context, task *wfbackend.WorkflowTask) error {
	return errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) CompleteWorkflowTask(ctx context.Context, task *wfbackend.WorkflowTask, state wfcore.WorkflowInstanceState, executedEvents, activityEvents, timerEvents []*history.Event, workflowEvents []*history.WorkflowEvent) error {
	return errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) GetActivityTask(ctx context.Context, queues []wflib.Queue) (*wfbackend.ActivityTask, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) ExtendActivityTask(ctx context.Context, task *wfbackend.ActivityTask) error {
	return errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) CompleteActivityTask(ctx context.Context, task *wfbackend.ActivityTask, result *history.Event) error {
	return errors.New("not implemented")
}

func (f *fakeWorkflowMonitoringBackend) GetStats(ctx context.Context) (*wfbackend.Stats, error) {
	return &wfbackend.Stats{}, nil
}

func (f *fakeWorkflowMonitoringBackend) Tracer() trace.Tracer {
	return trace.NewNoopTracerProvider().Tracer("test")
}

func (f *fakeWorkflowMonitoringBackend) Metrics() metrics.Client {
	return noopWorkflowMetrics{}
}

func (f *fakeWorkflowMonitoringBackend) Options() *wfbackend.Options {
	return wfbackend.ApplyOptions()
}

func (f *fakeWorkflowMonitoringBackend) Close() error {
	return nil
}

func (f *fakeWorkflowMonitoringBackend) FeatureSupported(feature wfbackend.Feature) bool {
	return true
}

type noopWorkflowMetrics struct{}

func (noopWorkflowMetrics) Counter(name string, tags metrics.Tags, value int64) {}

func (noopWorkflowMetrics) Distribution(name string, tags metrics.Tags, value float64) {}

func (noopWorkflowMetrics) Gauge(name string, tags metrics.Tags, value int64) {}

func (noopWorkflowMetrics) Timing(name string, tags metrics.Tags, duration time.Duration) {}

func (noopWorkflowMetrics) WithTags(tags metrics.Tags) metrics.Client {
	return noopWorkflowMetrics{}
}
