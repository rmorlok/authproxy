package helpers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	workflowworker "github.com/cschleiden/go-workflows/worker"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
	"github.com/stretchr/testify/require"
)

// StartCoreWorkflowWorker starts an in-test workflow worker registered with
// the core workflows. Tests call this before invoking API operations that
// return workflow-backed task ids, then poll the task endpoint.
func StartCoreWorkflowWorker(t *testing.T, env *IntegrationTestEnv) {
	t.Helper()

	workflowRuntime := env.DM.GetWorkflowRuntime()
	workflowWorker, err := apworkflows.NewWorker(workflowRuntime, &workflowworker.Options{
		WorkflowWorkerOptions: workflowworker.WorkflowWorkerOptions{
			WorkflowPollers:          2,
			MaxParallelWorkflowTasks: 1,
		},
		ActivityWorkerOptions: workflowworker.ActivityWorkerOptions{
			ActivityPollers:          2,
			MaxParallelActivityTasks: 1,
		},
	})
	require.NoError(t, err)
	require.NoError(t, env.Core.RegisterWorkflows(workflowWorker))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		if err := workflowWorker.Start(ctx); err != nil {
			errCh <- err
			return
		}
		errCh <- workflowWorker.WaitForCompletion()
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("workflow worker did not stop")
		}
	})
}

// RequireWorkflowTaskCompleted polls /tasks/{taskID} until the workflow task
// completes. It fails immediately if the task reaches the failed state.
func RequireWorkflowTaskCompleted(
	t *testing.T,
	env *IntegrationTestEnv,
	taskID string,
	timeout time.Duration,
	opts ...OAuth2Option,
) schemaapi.TaskInfoJson {
	t.Helper()

	cfg := env.resolveOAuth2Options(opts)
	var lastStatus int
	var lastBody string
	var lastState schemaapi.TaskState
	var result schemaapi.TaskInfoJson

	require.Eventually(t, func() bool {
		req, err := env.ApiAuthUtil.NewSignedRequestForActorExternalId(
			http.MethodGet,
			"/api/v1/tasks/"+taskID,
			nil,
			cfg.actorNamespace,
			cfg.actorExternalID,
			aschema.NoPermissions(),
		)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		if env.ApiGin != nil {
			env.ApiGin.ServeHTTP(w, req)
		} else {
			abs, err := url.Parse(env.ServerURL + "/api/v1/tasks/" + taskID)
			require.NoError(t, err)
			req.URL = abs
			req.Host = abs.Host
			req.RequestURI = ""
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			w.Code = resp.StatusCode
			for k, vs := range resp.Header {
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			if _, err := w.Body.ReadFrom(resp.Body); err != nil {
				require.NoError(t, err)
			}
		}

		lastStatus = w.Code
		lastBody = w.Body.String()
		if w.Code != http.StatusOK {
			return false
		}

		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		lastState = result.State
		require.Equal(t, taskID, result.Id)

		switch result.State {
		case schemaapi.TaskStateCompleted:
			return true
		case schemaapi.TaskStateFailed:
			t.Fatalf("workflow task failed: %s", w.Body.String())
		}
		return false
	}, timeout, 100*time.Millisecond,
		"workflow task should complete; last status=%d state=%s body=%s",
		lastStatus,
		lastState,
		lastBody,
	)

	return result
}

// RootActor returns helper options for the default integration-test actor.
func RootActor() OAuth2Option {
	return WithActor("test-actor", sconfig.RootNamespace)
}
