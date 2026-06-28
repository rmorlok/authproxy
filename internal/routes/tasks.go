package routes

import (
	"errors"
	"net/http"
	"reflect"
	"time"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	wfhistory "github.com/cschleiden/go-workflows/backend/history"
	wfcore "github.com/cschleiden/go-workflows/core"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	"github.com/rmorlok/authproxy/internal/tasks"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

type TaskRoutes struct {
	cfg            config.C
	auth           auth.A
	encrypt        encrypt.E
	asynqInspector apasynq.Inspector
	workflowClient apworkflows.Client
}

type TaskState = schemaapi.TaskState
type TaskInfoJson = schemaapi.TaskInfoJson
type OpenAPITaskInfoJson = schemaapiopenapi.TaskInfoJson

const (
	TaskStateUnknown   = schemaapi.TaskStateUnknown
	TaskStateActive    = schemaapi.TaskStateActive
	TaskStatePending   = schemaapi.TaskStatePending
	TaskStateScheduled = schemaapi.TaskStateScheduled
	TaskStateRetry     = schemaapi.TaskStateRetry
	TaskStateFailed    = schemaapi.TaskStateFailed
	TaskStateCompleted = schemaapi.TaskStateCompleted
)

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

func WorkflowTaskInfoToJson(encryptedId string, ti *tasks.TaskInfo, state wfcore.WorkflowInstanceState) *TaskInfoJson {
	ts := TaskStateUnknown
	switch state {
	case wfcore.WorkflowInstanceStateActive:
		ts = TaskStateActive
	case wfcore.WorkflowInstanceStateContinuedAsNew, wfcore.WorkflowInstanceStateFinished:
		ts = TaskStateCompleted
	}

	return &TaskInfoJson{
		Id:    encryptedId,
		Type:  ti.WorkflowName,
		State: ts,
	}
}

func WorkflowTaskInfoStateFromHistory(state wfcore.WorkflowInstanceState, historyEvents []*wfhistory.Event) TaskState {
	switch state {
	case wfcore.WorkflowInstanceStateActive:
		return TaskStateActive
	case wfcore.WorkflowInstanceStateContinuedAsNew:
		return TaskStateCompleted
	case wfcore.WorkflowInstanceStateFinished:
		for i := len(historyEvents) - 1; i >= 0; i-- {
			event := historyEvents[i]
			if event == nil {
				continue
			}
			switch event.Type {
			case wfhistory.EventType_WorkflowExecutionCanceled, wfhistory.EventType_WorkflowExecutionTerminated:
				return TaskStateFailed
			case wfhistory.EventType_WorkflowExecutionFinished:
				if workflowCompletionHasError(event.Attributes) {
					return TaskStateFailed
				}
				return TaskStateCompleted
			}
		}
		return TaskStateCompleted
	default:
		return TaskStateUnknown
	}
}

func workflowCompletionHasError(attrs any) bool {
	v := reflect.ValueOf(attrs)
	if !v.IsValid() {
		return false
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}

	errorField := v.FieldByName("Error")
	return errorField.IsValid() && !errorField.IsZero()
}

// @Summary		Get task status
// @Description	Get the status of a background task by its encrypted task info
// @Tags			tasks
// @Accept			json
// @Produce		json
// @Param			encryptedTaskInfo	path		string	true	"Encrypted task info token"
// @Success		200					{object}	OpenAPITaskInfoJson
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

	if ti.ActorId != apid.Nil && ti.ActorId != ra.MustGetActor().Id {
		apgin.WriteError(gctx, nil, httperr.Forbidden("not authorized to view task"))
		return
	}

	if ti.TrackedVia == tasks.TrackedViaWorkflow {
		if r.workflowClient == nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("workflow task tracking is not configured"))
			return
		}
		if ti.WorkflowInstanceId == "" || ti.WorkflowExecutionId == "" || ti.WorkflowName == "" {
			apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("invalid task info: data missing"))
			return
		}

		instance := wfcore.NewWorkflowInstance(ti.WorkflowInstanceId, ti.WorkflowExecutionId)
		state, err := r.workflowClient.GetWorkflowInstanceState(ctx, instance)
		if err != nil {
			if errors.Is(err, wfbackend.ErrInstanceNotFound) {
				apgin.WriteError(gctx, nil, httperr.NotFound("task not found"))
				return
			}

			apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to load task", httperr.WithInternalErr(err)))
			return
		}

		var historyEvents []*wfhistory.Event
		if state == wfcore.WorkflowInstanceStateFinished {
			historyEvents, err = r.workflowClient.GetWorkflowInstanceHistory(ctx, instance, nil)
			if err != nil {
				if errors.Is(err, wfbackend.ErrInstanceNotFound) {
					apgin.WriteError(gctx, nil, httperr.NotFound("task not found"))
					return
				}

				apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to load task", httperr.WithInternalErr(err)))
				return
			}
		}

		resp := WorkflowTaskInfoToJson(encryptedTaskInfo, ti, state)
		resp.State = WorkflowTaskInfoStateFromHistory(state, historyEvents)
		apgin.APIJSON(gctx, http.StatusOK, resp)
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

	ati, err := r.asynqInspector.GetTaskInfo(ti.AsynqQueue, ti.AsynqId)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("task not found"))
			return
		}

		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to load task", httperr.WithInternalErr(err)))
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, TaskInfoToJson(encryptedTaskInfo, ati))
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
	workflowClient apworkflows.Client,
) *TaskRoutes {
	return &TaskRoutes{
		cfg:            cfg,
		auth:           authService,
		encrypt:        encrypt,
		asynqInspector: asynqInspector,
		workflowClient: workflowClient,
	}
}
