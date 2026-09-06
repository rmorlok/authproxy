package routes

import (
	"errors"
	"net/http"
	"strconv"

	wfbackend "github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/history"
	wfcore "github.com/cschleiden/go-workflows/core"
	"github.com/cschleiden/go-workflows/diag"
	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
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
	AfterInstanceID  string `json:"afterInstanceId"`
	AfterExecutionID string `json:"afterExecutionId"`
	Count            int    `json:"count"`
}

func workflowInstanceFromParams(gctx *gin.Context) (*wfcore.WorkflowInstance, bool) {
	instanceID := gctx.Param("instanceId")
	executionID := gctx.Param("executionId")
	if instanceID == "" || executionID == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("instanceId and executionId are required"))
		return nil, false
	}

	return wfcore.NewWorkflowInstance(instanceID, executionID), true
}

func workflowInstanceReferenceToJson(instance *wfcore.WorkflowInstance) *schemaapi.WorkflowInstanceReferenceJson {
	if instance == nil {
		return nil
	}

	return &schemaapi.WorkflowInstanceReferenceJson{
		Target: meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       schemaapi.WorkflowInstanceKind,
			ID:         instance.ExecutionID,
		},
		InstanceID: instance.InstanceID,
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

func workflowInstanceRefToJson(ref *diag.WorkflowInstanceRef) *schemaapi.WorkflowInstanceJson {
	if ref == nil || ref.Instance == nil {
		return nil
	}

	return &schemaapi.WorkflowInstanceJson{
		TypeMeta: meta.NewTypeMeta(schemaapi.WorkflowInstanceKind),
		Metadata: meta.ObjectMeta{
			ID:        ref.Instance.ExecutionID,
			CreatedAt: util.UtcTimePointer(util.ToPtrNonZero(ref.CreatedAt)),
		},
		Spec: schemaapi.WorkflowInstanceSpecJson{
			InstanceID: ref.Instance.InstanceID,
			Queue:      ref.Queue,
			ParentRef:  workflowInstanceReferenceToJson(ref.Instance.Parent),
		},
		Status: schemaapi.WorkflowInstanceStatusJson{
			State:       workflowInstanceStateToJson(ref.State),
			CompletedAt: util.UtcTimePointer(ref.CompletedAt),
		},
	}
}

func workflowInstanceRefsToJson(refs []*diag.WorkflowInstanceRef) []schemaapi.WorkflowInstanceJson {
	result := make([]schemaapi.WorkflowInstanceJson, 0, len(refs))
	for _, ref := range refs {
		if item := workflowInstanceRefToJson(ref); item != nil {
			result = append(result, *item)
		}
	}
	return result
}

func workflowHistoryEventsToJson(events []*history.Event) []schemaapi.WorkflowHistoryEventJson {
	result := make([]schemaapi.WorkflowHistoryEventJson, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		result = append(result, schemaapi.WorkflowHistoryEventJson{
			TypeMeta: meta.NewTypeMeta(schemaapi.WorkflowHistoryEventKind),
			Metadata: meta.ObjectMeta{
				ID:        event.ID,
				CreatedAt: util.UtcTimePointer(util.ToPtrNonZero(event.Timestamp)),
			},
			Spec: schemaapi.WorkflowHistoryEventSpecJson{
				SequenceID:      event.SequenceID,
				Type:            event.Type.String(),
				ScheduleEventID: event.ScheduleEventID,
				Attributes:      event.Attributes,
				VisibleAt:       util.UtcTimePointer(event.VisibleAt),
			},
		})
	}

	return result
}

func workflowInstanceTreeToJson(tree *diag.WorkflowInstanceTree) *schemaapi.WorkflowInstanceJson {
	if tree == nil {
		return nil
	}

	children := make([]schemaapi.WorkflowInstanceJson, 0, len(tree.Children))
	for _, child := range tree.Children {
		if item := workflowInstanceTreeToJson(child); item != nil {
			children = append(children, *item)
		}
	}

	result := workflowInstanceRefToJson(tree.WorkflowInstanceRef)
	if result == nil {
		return nil
	}
	result.Spec.WorkflowName = tree.WorkflowName
	result.Status.Error = tree.Error
	result.Status.Children = children
	return result
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

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListWorkflowInstancesResponseJson(
		workflowInstanceRefsToJson(items), cursor,
	))
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

	response := workflowInstanceRefToJson(instanceRef)
	response.Status.History = workflowHistoryEventsToJson(historyEvents)
	apgin.APIJSON(gctx, http.StatusOK, response)
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

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListWorkflowHistoryResponseJson(
		workflowHistoryEventsToJson(historyEvents),
	))
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

	apgin.APIJSON(gctx, http.StatusOK, workflowInstanceTreeToJson(tree))
}

func (r *WorkflowMonitoringRoutes) cancelInstance(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	instance, ok := workflowInstanceFromParams(gctx)
	if !ok {
		return
	}

	if err := r.backend.CancelWorkflowInstance(ctx, instance, history.NewWorkflowCancellationEvent(apctx.GetClock(ctx).Now())); err != nil {
		writeWorkflowBackendError(gctx, "failed to cancel workflow instance", err)
		return
	}

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewWorkflowInstanceActionResponse(
		schemaapi.WorkflowCancelActionKind, instance.InstanceID, instance.ExecutionID,
	))
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

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewWorkflowInstanceActionResponse(
		schemaapi.WorkflowDeleteActionKind, instance.InstanceID, instance.ExecutionID,
	))
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
		"/workflow-monitoring/instances/:instanceId/:executionId",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("get").
			Build(),
		r.getInstance,
	)
	g.GET(
		"/workflow-monitoring/instances/:instanceId/:executionId/history",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("get").
			Build(),
		r.getHistory,
	)
	g.GET(
		"/workflow-monitoring/instances/:instanceId/:executionId/tree",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("get").
			Build(),
		r.getTree,
	)
	g.POST(
		"/workflow-monitoring/instances/:instanceId/:executionId/_cancel",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("manage").
			Build(),
		r.cancelInstance,
	)
	g.DELETE(
		"/workflow-monitoring/instances/:instanceId/:executionId",
		r.auth.NewRequiredBuilder().
			ForResource("workflow_monitoring").
			ForVerb("manage").
			Build(),
		r.removeInstance,
	)
}
