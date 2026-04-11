package routes

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apgin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/tasks"
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

// @Summary		Get task status
// @Description	Get the status of a background task by its encrypted task info
// @Tags			tasks
// @Accept			json
// @Produce		json
// @Param			encryptedTaskInfo	path		string	true	"Encrypted task info token"
// @Success		200					{object}	TaskInfoJson
// @Failure		400					{object}	ErrorResponse
// @Failure		401					{object}	ErrorResponse
// @Failure		403					{object}	ErrorResponse
// @Failure		404					{object}	ErrorResponse
// @Failure		500					{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/tasks/{encryptedTaskInfo} [get]
func (r *TaskRoutes) get(gctx *gin.Context) {
	ctx := gctx.Request.Context()

	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		apgin.WriteError(gctx, nil, httperr.UnauthorizedMsg("unauthorized"))
		return
	}

	encryptedTaskInfo := gctx.Param("encryptedTaskInfo")
	if encryptedTaskInfo == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
	}

	ti, err := tasks.FromSecureEncryptedString(ctx, r.encrypt, encryptedTaskInfo)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid task info", httperr.WithInternalErr(err)))
		return
	}

	// This really shouldn't happen
	if ti == nil {
		apgin.WriteError(gctx, nil, httperr.NotFound("connection not found"))
		return
	}

	if ti.TrackedVia != tasks.TrackedViaAsynq {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("invalid task info: bad type"))
		return
	}

	if ti.AsynqQueue == "" || ti.AsynqId == "" {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("invalid task info: data missing"))
		return
	}

	if ti.ActorId != apid.Nil && ti.ActorId != ra.MustGetActor().Id {
		apgin.WriteError(gctx, nil, httperr.Forbidden("not authorized to view task"))
		return
	}

	ati, err := r.asynqInspector.GetTaskInfo(ti.AsynqQueue, ti.AsynqId)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("task not found"))
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to load task", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, TaskInfoToJson(encryptedTaskInfo, ati))
}

func (r *TaskRoutes) Register(g gin.IRouter) {
	g.GET(
		"/tasks/:encryptedTaskInfo",
		r.auth.Required(),
		r.get,
	) // Not covered by permissions because of encrypted field-based permission checking
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
