package routes

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/tasks"
	"net/http"
	"time"
)

type TaskRoutes struct {
	cfg            config.C
	auth           auth.A
	encrypt        encrypt.E
	asynqInspector apasynq.Inspector
}

type TaskState string

const (
	// TaskStateUnknown indicates that the state was unable to be determined.
	TaskStateUnknown TaskState = "unknown"

	// TaskStateActive indicates that the task is currently being processed by Handler.
	TaskStateActive TaskState = "active"

	// TaskStatePending indicates that the task is ready to be processed by Handler.
	TaskStatePending = "pending"

	// TaskStateScheduled indicates that the task is scheduled to be processed some time in the future.
	TaskStateScheduled = "scheduled"

	// TaskStateRetry indicates that the task has previously failed and scheduled to be processed some time in the future.
	TaskStateRetry = "retry"

	// TaskStateArchived indicates that the task exhausted retries
	TaskStateFailed = "failed"

	// TaskStateCompleted indicates that the task is processed successfully and retained until the retention TTL expires.
	TaskStateCompleted = "completed"
)

type TaskInfoJson struct {
	Id        string     `json:"id"`
	Type      string     `json:"type"`
	State     TaskState  `json:"state"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func TaskInfoToJson(encryptedId string, ti *asynq.TaskInfo) *TaskInfoJson {
	ts := TaskStateUnknown
	switch ti.State {
	case asynq.TaskStateActive:
		ts = TaskStateActive
	case asynq.TaskStatePending:
		ts = TaskStatePending
	case asynq.TaskStateScheduled:
		ts = TaskStateScheduled
	case asynq.TaskStateRetry:
		ts = TaskStateRetry
	case asynq.TaskStateArchived:
		// Archived implies that retries were exhausted. See documentation:
		// https://github.com/hibiken/asynq/wiki/Life-of-a-Task
		ts = TaskStateFailed
	case asynq.TaskStateCompleted:
		ts = TaskStateCompleted
	case asynq.TaskStateAggregating:
		// This isn't something we need to expose to clients. Just flag the task as pending work.
		ts = TaskStatePending
	}

	updatedAt := time.Time{}
	if ti.LastFailedAt.After(updatedAt) {
		updatedAt = ti.LastFailedAt
	}
	if ti.CompletedAt.After(updatedAt) {
		updatedAt = ti.CompletedAt
	}

	var updatedAtPtr *time.Time
	if !updatedAt.IsZero() {
		updatedAtPtr = &updatedAt
	}

	return &TaskInfoJson{
		Id:        encryptedId,
		Type:      ti.Type,
		State:     ts,
		UpdatedAt: updatedAtPtr,
	}
}

func (r *TaskRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			WithResponseMsg("unauthorized").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	encryptedTaskInfo := gctx.Param("encryptedTaskInfo")
	if encryptedTaskInfo == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("id is required").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
	}

	ti, err := tasks.FromSecureEncryptedString(ctx, r.encrypt, encryptedTaskInfo)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithInternalErr(err).
			WithResponseMsg("invalid task info").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// This really shouldn't happen
	if ti == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("connection not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if ti.TrackedVia != tasks.TrackedViaAsynq {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithResponseMsg("invalid task info: bad type").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if ti.AsynqQueue == "" || ti.AsynqId == "" {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithResponseMsg("invalid task info: data missing").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if ti.ActorId != uuid.Nil && ti.ActorId != ra.MustGetActor().Id {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusForbidden().
			WithResponseMsg("not authorized to view task").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	ati, err := r.asynqInspector.GetTaskInfo(ti.AsynqQueue, ti.AsynqId)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskNotFound) {
			api_common.NewHttpStatusErrorBuilder().
				WithStatusNotFound().
				WithResponseMsg("task not found").
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}

		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			WithResponseMsg("failed to load task").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	gctx.PureJSON(http.StatusOK, TaskInfoToJson(encryptedTaskInfo, ati))
}

func (r *TaskRoutes) Register(g gin.IRouter) {
	g.GET("/tasks/:encryptedTaskInfo", r.auth.Required(), r.get)
}

func NewTaskRoutes(
	cfg config.C,
	authService auth.A,
	encrypt encrypt.E,
	asynqInspector apasynq.Inspector,
) *TaskRoutes {
	return &TaskRoutes{
		cfg:            cfg,
		auth:           authService,
		encrypt:        encrypt,
		asynqInspector: asynqInspector,
	}
}
