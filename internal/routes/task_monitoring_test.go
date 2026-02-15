package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestTaskMonitoringRoutes(t *testing.T) {
	type TestSetup struct {
		Gin           *gin.Engine
		AuthUtil      *auth2.AuthTestUtil
		MockInspector *mock.MockInspector
	}

	setup := func(t *testing.T, cfg config.C) *TestSetup {
		ctrl := gomock.NewController(t)
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)

		inspector := mock.NewMockInspector(ctrl)
		routes := NewTaskMonitoringRoutes(cfg, auth, inspector)

		r := api_common.GinForTest(nil)
		routes.Register(r)

		return &TestSetup{
			Gin:           r,
			AuthUtil:      authUtil,
			MockInspector: inspector,
		}
	}

	t.Run("listQueues", func(t *testing.T) {
		t.Run("unauthorized", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/task-monitoring/queues", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				Queues().
				Return([]string{"default"}, nil)

			tu.MockInspector.EXPECT().
				GetQueueInfo("default").
				Return(&asynq.QueueInfo{
					Queue:     "default",
					Pending:   5,
					Active:    2,
					Scheduled: 1,
					Retry:     0,
					Archived:  3,
					Completed: 10,
					Paused:    false,
					Timestamp: time.Now().UTC(),
				}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp []*QueueInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp, 1)
			require.Equal(t, "default", resp[0].Queue)
			require.Equal(t, 5, resp[0].Pending)
			require.Equal(t, 2, resp[0].Active)
		})

		t.Run("inspector error", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				Queues().
				Return(nil, errors.New("redis error"))

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})

	t.Run("getQueueInfo", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "get"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				GetQueueInfo("default").
				Return(&asynq.QueueInfo{
					Queue:     "default",
					Pending:   3,
					Paused:    true,
					Timestamp: time.Now().UTC(),
				}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp QueueInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "default", resp.Queue)
			require.Equal(t, 3, resp.Pending)
			require.True(t, resp.Paused)
		})
	})

	t.Run("getQueueHistory", func(t *testing.T) {
		t.Run("success default days", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default/history",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "get"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				History("default", 30).
				Return([]*asynq.DailyStats{
					{Queue: "default", Processed: 100, Failed: 5, Date: time.Now().UTC()},
				}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp []*DailyStatsJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp, 1)
			require.Equal(t, 100, resp[0].Processed)
			require.Equal(t, 5, resp[0].Failed)
		})

		t.Run("custom days", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default/history?days=7",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "get"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				History("default", 7).
				Return([]*asynq.DailyStats{}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("invalid days", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default/history?days=abc",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "get"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("listTasksByState", func(t *testing.T) {
		t.Run("pending tasks", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default/tasks/pending",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				ListPendingTasks("default", gomock.Any(), gomock.Any()).
				Return([]*asynq.TaskInfo{
					{
						ID:       "task-1",
						Queue:    "default",
						Type:     "email:send",
						Payload:  []byte(`{"to":"user@example.com"}`),
						State:    asynq.TaskStatePending,
						MaxRetry: 3,
						Retried:  0,
					},
				}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp []*MonitoringTaskInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp, 1)
			require.Equal(t, "task-1", resp[0].ID)
			require.Equal(t, "email:send", resp[0].Type)
			require.Equal(t, "pending", resp[0].State)
			require.Equal(t, `{"to":"user@example.com"}`, resp[0].Payload)
		})

		t.Run("invalid state", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default/tasks/invalid",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("inspector error", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default/tasks/active",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				ListActiveTasks("default", gomock.Any(), gomock.Any()).
				Return(nil, errors.New("redis error"))

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})

	t.Run("getTask", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/queues/default/tasks/pending/task-123",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "get"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				GetTaskInfo("default", "task-123").
				Return(&asynq.TaskInfo{
					ID:       "task-123",
					Queue:    "default",
					Type:     "email:send",
					Payload:  []byte(`{"to":"test@example.com"}`),
					State:    asynq.TaskStatePending,
					MaxRetry: 5,
				}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp MonitoringTaskInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, "task-123", resp.ID)
			require.Equal(t, "email:send", resp.Type)
		})
	})

	t.Run("listServers", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/servers",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				Servers().
				Return([]*asynq.ServerInfo{
					{
						ID:          "server-1",
						Host:        "localhost",
						PID:         1234,
						Concurrency: 10,
						Queues:      map[string]int{"default": 1},
						Status:      "active",
						Started:     time.Now().UTC(),
						ActiveWorkers: []*asynq.WorkerInfo{
							{
								TaskID:   "task-1",
								TaskType: "email:send",
								Queue:    "default",
								Started:  time.Now().UTC(),
								Deadline: time.Now().Add(time.Hour).UTC(),
							},
						},
					},
				}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp []*ServerInfoJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp, 1)
			require.Equal(t, "server-1", resp[0].ID)
			require.Equal(t, 10, resp[0].Concurrency)
			require.Len(t, resp[0].ActiveWorkers, 1)
		})
	})

	t.Run("listSchedulerEntries", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/task-monitoring/scheduler-entries",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				SchedulerEntries().
				Return([]*asynq.SchedulerEntry{
					{
						ID:   "entry-1",
						Spec: "*/5 * * * *",
						Task: asynq.NewTask("probe:check", nil),
						Next: time.Now().Add(5 * time.Minute).UTC(),
						Prev: time.Now().Add(-5 * time.Minute).UTC(),
					},
				}, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp []*SchedulerEntryJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp, 1)
			require.Equal(t, "entry-1", resp[0].ID)
			require.Equal(t, "*/5 * * * *", resp[0].Spec)
			require.Equal(t, "probe:check", resp[0].TaskType)
		})
	})

	t.Run("manage endpoints", func(t *testing.T) {
		t.Run("runTask success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/tasks/task-1/_run",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				RunTask("default", "task-1").
				Return(nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("runTask error", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/tasks/task-1/_run",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				RunTask("default", "task-1").
				Return(errors.New("task not found"))

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusInternalServerError, w.Code)
		})

		t.Run("runTask forbidden with list verb", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/tasks/task-1/_run",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "list"),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("archiveTask success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/tasks/task-1/_archive",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				ArchiveTask("default", "task-1").
				Return(nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("cancelTask success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/tasks/task-1/_cancel",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				CancelProcessing("task-1").
				Return(nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("deleteTask success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/task-monitoring/queues/default/tasks/task-1",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				DeleteTask("default", "task-1").
				Return(nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("pauseQueue success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/_pause",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				PauseQueue("default").
				Return(nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("unpauseQueue success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/_unpause",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				UnpauseQueue("default").
				Return(nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("runAllArchivedTasks success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/archived/_run-all",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				RunAllArchivedTasks("default").
				Return(5, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp BulkActionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, 5, resp.AffectedCount)
		})

		t.Run("runAllRetryTasks success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/task-monitoring/queues/default/retry/_run-all",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				RunAllRetryTasks("default").
				Return(3, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp BulkActionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, 3, resp.AffectedCount)
		})

		t.Run("deleteAllArchivedTasks success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/task-monitoring/queues/default/archived",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				DeleteAllArchivedTasks("default").
				Return(7, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp BulkActionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, 7, resp.AffectedCount)
		})

		t.Run("deleteAllCompletedTasks success", func(t *testing.T) {
			tu := setup(t, nil)

			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodDelete,
				"/task-monitoring/queues/default/completed",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "task_monitoring", "manage"),
			)
			require.NoError(t, err)

			tu.MockInspector.EXPECT().
				DeleteAllCompletedTasks("default").
				Return(12, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp BulkActionResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Equal(t, 12, resp.AffectedCount)
		})
	})
}
