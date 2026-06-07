package routes

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/history"
	wfcore "github.com/cschleiden/go-workflows/core"
	"github.com/cschleiden/go-workflows/diag"
	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type WorkflowMonitoringRoutes struct {
	auth            auth.A
	backend         diag.Backend
	cursorEncryptor pagination.CursorEncryptor
}

func NewWorkflowMonitoringRoutes(
	auth auth.A,
	backend diag.Backend,
	cursorEncryptor pagination.CursorEncryptor,
) *WorkflowMonitoringRoutes {
	return &WorkflowMonitoringRoutes{
		auth:            auth,
		backend:         backend,
		cursorEncryptor: cursorEncryptor,
	}
}

type workflowListCursor struct {
	AfterInstanceID  string `json:"after_instance_id"`
	AfterExecutionID string `json:"after_execution_id"`
	Count            int    `json:"count"`
}

type WorkflowInstanceRefJson = schemaapi.WorkflowInstanceRefJson
type WorkflowHistoryEventJson = schemaapi.WorkflowHistoryEventJson
type WorkflowInstanceInfoJson = schemaapi.WorkflowInstanceInfoJson
type WorkflowInstanceTreeJson = schemaapi.WorkflowInstanceTreeJson
type ListWorkflowInstancesResponseJson = schemaapi.ListWorkflowInstancesResponseJson
type ListWorkflowHistoryResponseJson = schemaapi.ListWorkflowHistoryResponseJson

func workflowInstanceFromParams(gctx *gin.Context) (*wfcore.WorkflowInstance, bool) {
	instanceID := gctx.Param("instance_id")
	executionID := gctx.Param("execution_id")
	if instanceID == "" || executionID == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("instance_id and execution_id are required"))
		return nil, false
	}

	return wfcore.NewWorkflowInstance(instanceID, executionID), true
}

func workflowInstanceToJson(instance *wfcore.WorkflowInstance) *schemaapi.WorkflowInstanceJson {
	if instance == nil {
		return nil
	}

	return &schemaapi.WorkflowInstanceJson{
		InstanceID:  instance.InstanceID,
		ExecutionID: instance.ExecutionID,
		Parent:      workflowInstanceToJson(instance.Parent),
	}
}

func workflowInstanceStateToJson(state wfcore.WorkflowInstanceState) string {
	switch state {
	case wfcore.WorkflowInstanceStateActive:
		return "active"
	case wfcore.WorkflowInstanceStateContinuedAsNew:
		return "continued_as_new"
	case wfcore.WorkflowInstanceStateFinished:
		return "finished"
	default:
		return "unknown"
	}
}

func workflowInstanceRefToJson(ref *diag.WorkflowInstanceRef) *WorkflowInstanceRefJson {
	if ref == nil {
		return nil
	}

	return &WorkflowInstanceRefJson{
		Instance:    workflowInstanceToJson(ref.Instance),
		CreatedAt:   ref.CreatedAt,
		CompletedAt: ref.CompletedAt,
		State:       workflowInstanceStateToJson(ref.State),
		Queue:       ref.Queue,
	}
}

func workflowInstanceRefsToJson(refs []*diag.WorkflowInstanceRef) []*WorkflowInstanceRefJson {
	result := make([]*WorkflowInstanceRefJson, 0, len(refs))
	for _, ref := range refs {
		result = append(result, workflowInstanceRefToJson(ref))
	}
	return result
}

func workflowHistoryEventsToJson(events []*history.Event) []*WorkflowHistoryEventJson {
	result := make([]*WorkflowHistoryEventJson, 0, len(events))
	for _, event := range events {
		result = append(result, &WorkflowHistoryEventJson{
			ID:              event.ID,
			SequenceID:      event.SequenceID,
			Type:            event.Type.String(),
			Timestamp:       event.Timestamp,
			ScheduleEventID: event.ScheduleEventID,
			Attributes:      event.Attributes,
			VisibleAt:       event.VisibleAt,
		})
	}

	return result
}

func workflowInstanceTreeToJson(tree *diag.WorkflowInstanceTree) *WorkflowInstanceTreeJson {
	if tree == nil {
		return nil
	}

	children := make([]*WorkflowInstanceTreeJson, 0, len(tree.Children))
	for _, child := range tree.Children {
		children = append(children, workflowInstanceTreeToJson(child))
	}

	return &WorkflowInstanceTreeJson{
		WorkflowInstanceRefJson: workflowInstanceRefToJson(tree.WorkflowInstanceRef),
		WorkflowName:            tree.WorkflowName,
		Error:                   tree.Error,
		Children:                children,
	}
}

func writeWorkflowBackendError(gctx *gin.Context, publicMessage string, err error) {
	switch {
	case errors.Is(err, wfbackend.ErrInstanceNotFound):
		apgin.WriteError(gctx, nil, httperr.NotFound("workflow instance not found", httperr.WithInternalErr(err)))
	case errors.Is(err, wfbackend.ErrInstanceNotFinished):
		apgin.WriteError(gctx, nil, httperr.Conflict("workflow instance is not finished", httperr.WithInternalErr(err)))
	default:
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg(publicMessage, httperr.WithInternalErr(err)))
	}
}

func (r *WorkflowMonitoringRoutes) listInstances(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	count := 25
	afterInstanceID := ""
	afterExecutionID := ""

	if cursorStr := gctx.Query("cursor"); cursorStr != "" {
		cursor, err := pagination.ParseCursor[workflowListCursor](ctx, r.cursorEncryptor, cursorStr)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid cursor"))
			return
		}
		count = cursor.Count
		afterInstanceID = cursor.AfterInstanceID
		afterExecutionID = cursor.AfterExecutionID
	} else if countStr := gctx.Query("limit"); countStr != "" {
		parsed, err := strconv.Atoi(countStr)
		if err != nil || parsed < 1 {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid limit parameter"))
			return
		}
		count = parsed
	}

	if count > 100 {
		count = 100
	}

	items, err := r.backend.GetWorkflowInstances(ctx, afterInstanceID, afterExecutionID, count+1)
	if err != nil {
		writeWorkflowBackendError(gctx, "failed to list workflow instances", err)
		return
	}

	var cursor string
	if len(items) > count {
		items = items[:count]
		last := items[len(items)-1]
		if last != nil && last.Instance != nil {
			cursor, _ = pagination.MakeCursor(ctx, r.cursorEncryptor, &workflowListCursor{
				AfterInstanceID:  last.Instance.InstanceID,
				AfterExecutionID: last.Instance.ExecutionID,
				Count:            count,
			})
		}
	}

	gctx.PureJSON(http.StatusOK, ListWorkflowInstancesResponseJson{
		Items:  workflowInstanceRefsToJson(items),
		Cursor: cursor,
	})
}

func (r *WorkflowMonitoringRoutes) getInstance(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	instance, ok := workflowInstanceFromParams(gctx)
	if !ok {
		return
	}

	instanceRef, err := r.backend.GetWorkflowInstance(ctx, instance)
	if err != nil {
		writeWorkflowBackendError(gctx, "failed to get workflow instance", err)
		return
	}
	if instanceRef == nil {
		apgin.WriteError(gctx, nil, httperr.NotFound("workflow instance not found"))
		return
	}

	historyEvents, err := r.backend.GetWorkflowInstanceHistory(ctx, instanceRef.Instance, nil)
	if err != nil {
		writeWorkflowBackendError(gctx, "failed to get workflow history", err)
		return
	}

	gctx.PureJSON(http.StatusOK, &WorkflowInstanceInfoJson{
		WorkflowInstanceRefJson: workflowInstanceRefToJson(instanceRef),
		History:                 workflowHistoryEventsToJson(historyEvents),
	})
}

func (r *WorkflowMonitoringRoutes) getHistory(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	instance, ok := workflowInstanceFromParams(gctx)
	if !ok {
		return
	}

	instanceRef, err := r.backend.GetWorkflowInstance(ctx, instance)
	if err != nil {
		writeWorkflowBackendError(gctx, "failed to get workflow instance", err)
		return
	}
	if instanceRef == nil {
		apgin.WriteError(gctx, nil, httperr.NotFound("workflow instance not found"))
		return
	}

	historyEvents, err := r.backend.GetWorkflowInstanceHistory(ctx, instanceRef.Instance, nil)
	if err != nil {
		writeWorkflowBackendError(gctx, "failed to get workflow history", err)
		return
	}

	gctx.PureJSON(http.StatusOK, ListWorkflowHistoryResponseJson{Items: workflowHistoryEventsToJson(historyEvents)})
}

func (r *WorkflowMonitoringRoutes) getTree(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	instance, ok := workflowInstanceFromParams(gctx)
	if !ok {
		return
	}

	tree, err := r.backend.GetWorkflowTree(ctx, instance)
	if err != nil {
		writeWorkflowBackendError(gctx, "failed to get workflow tree", err)
		return
	}
	if tree == nil {
		apgin.WriteError(gctx, nil, httperr.NotFound("workflow instance tree not found"))
		return
	}

	gctx.PureJSON(http.StatusOK, workflowInstanceTreeToJson(tree))
}

func (r *WorkflowMonitoringRoutes) cancelInstance(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	instance, ok := workflowInstanceFromParams(gctx)
	if !ok {
		return
	}

	if err := r.backend.CancelWorkflowInstance(ctx, instance, history.NewWorkflowCancellationEvent(time.Now())); err != nil {
		writeWorkflowBackendError(gctx, "failed to cancel workflow instance", err)
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *WorkflowMonitoringRoutes) removeInstance(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	instance, ok := workflowInstanceFromParams(gctx)
	if !ok {
		return
	}

	if err := r.backend.RemoveWorkflowInstance(ctx, instance); err != nil {
		writeWorkflowBackendError(gctx, "failed to remove workflow instance", err)
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *WorkflowMonitoringRoutes) Register(g gin.IRouter) {
	g.GET(
		"/workflow-monitoring/instances",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("list").
			Build(),
		r.listInstances,
	)
	g.GET(
		"/workflow-monitoring/instances/:instance_id/:execution_id",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("get").
			Build(),
		r.getInstance,
	)
	g.GET(
		"/workflow-monitoring/instances/:instance_id/:execution_id/history",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("get").
			Build(),
		r.getHistory,
	)
	g.GET(
		"/workflow-monitoring/instances/:instance_id/:execution_id/tree",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("get").
			Build(),
		r.getTree,
	)
	g.POST(
		"/workflow-monitoring/instances/:instance_id/:execution_id/_cancel",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("manage").
			Build(),
		r.cancelInstance,
	)
	g.DELETE(
		"/workflow-monitoring/instances/:instance_id/:execution_id",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("manage").
			Build(),
		r.removeInstance,
	)
}
