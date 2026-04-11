package routes

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/apasynq"
	"github.com/rmorlok/authproxy/internal/apgin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type TaskMonitoringRoutes struct {
	cfg             config.C
	auth            auth.A
	inspector       apasynq.Inspector
	cursorEncryptor pagination.CursorEncryptor
}

func NewTaskMonitoringRoutes(
	cfg config.C,
	auth auth.A,
	inspector apasynq.Inspector,
	cursorEncryptor pagination.CursorEncryptor,
) *TaskMonitoringRoutes {
	return &TaskMonitoringRoutes{
		cfg:             cfg,
		auth:            auth,
		inspector:       inspector,
		cursorEncryptor: cursorEncryptor,
	}
}

// JSON response models

type QueueInfoJson struct {
	Queue          string  `json:"queue"`
	MemoryUsage    int64   `json:"memory_usage"`
	Latency        float64 `json:"latency_seconds"`
	Size           int     `json:"size"`
	Groups         int     `json:"groups"`
	Pending        int     `json:"pending"`
	Active         int     `json:"active"`
	Scheduled      int     `json:"scheduled"`
	Retry          int     `json:"retry"`
	Archived       int     `json:"archived"`
	Completed      int     `json:"completed"`
	Aggregating    int     `json:"aggregating"`
	Processed      int     `json:"processed"`
	Failed         int     `json:"failed"`
	ProcessedTotal int     `json:"processed_total"`
	FailedTotal    int     `json:"failed_total"`
	Paused         bool    `json:"paused"`
	Timestamp      string  `json:"timestamp"`
}

func queueInfoToJson(qi *asynq.QueueInfo) *QueueInfoJson {
	return &QueueInfoJson{
		Queue:          qi.Queue,
		MemoryUsage:    qi.MemoryUsage,
		Latency:        qi.Latency.Seconds(),
		Size:           qi.Size,
		Groups:         qi.Groups,
		Pending:        qi.Pending,
		Active:         qi.Active,
		Scheduled:      qi.Scheduled,
		Retry:          qi.Retry,
		Archived:       qi.Archived,
		Completed:      qi.Completed,
		Aggregating:    qi.Aggregating,
		Processed:      qi.Processed,
		Failed:         qi.Failed,
		ProcessedTotal: qi.ProcessedTotal,
		FailedTotal:    qi.FailedTotal,
		Paused:         qi.Paused,
		Timestamp:      qi.Timestamp.UTC().Format(time.RFC3339),
	}
}

type MonitoringTaskInfoJson struct {
	ID            string `json:"id"`
	Queue         string `json:"queue"`
	Type          string `json:"type"`
	Payload       string `json:"payload"`
	State         string `json:"state"`
	MaxRetry      int    `json:"max_retry"`
	Retried       int    `json:"retried"`
	LastErr       string `json:"last_err,omitempty"`
	LastFailedAt  string `json:"last_failed_at,omitempty"`
	NextProcessAt string `json:"next_process_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	IsOrphaned    bool   `json:"is_orphaned,omitempty"`
	Group         string `json:"group,omitempty"`
}

func taskInfoToJson(ti *asynq.TaskInfo) *MonitoringTaskInfoJson {
	payload := base64.StdEncoding.EncodeToString(ti.Payload)
	if json.Valid(ti.Payload) {
		payload = string(ti.Payload)
	}

	j := &MonitoringTaskInfoJson{
		ID:       ti.ID,
		Queue:    ti.Queue,
		Type:     ti.Type,
		Payload:  payload,
		State:    ti.State.String(),
		MaxRetry: ti.MaxRetry,
		Retried:  ti.Retried,
		LastErr:  ti.LastErr,
		Group:    ti.Group,
	}

	if !ti.LastFailedAt.IsZero() {
		j.LastFailedAt = ti.LastFailedAt.UTC().Format(time.RFC3339)
	}
	if !ti.NextProcessAt.IsZero() {
		j.NextProcessAt = ti.NextProcessAt.UTC().Format(time.RFC3339)
	}
	if !ti.CompletedAt.IsZero() {
		j.CompletedAt = ti.CompletedAt.UTC().Format(time.RFC3339)
	}
	if ti.IsOrphaned {
		j.IsOrphaned = true
	}

	return j
}

type DailyStatsJson struct {
	Queue     string `json:"queue"`
	Processed int    `json:"processed"`
	Failed    int    `json:"failed"`
	Date      string `json:"date"`
}

func dailyStatsToJson(ds *asynq.DailyStats) *DailyStatsJson {
	return &DailyStatsJson{
		Queue:     ds.Queue,
		Processed: ds.Processed,
		Failed:    ds.Failed,
		Date:      ds.Date.UTC().Format("2006-01-02"),
	}
}

type WorkerInfoJson struct {
	TaskID   string `json:"task_id"`
	TaskType string `json:"task_type"`
	Queue    string `json:"queue"`
	Started  string `json:"started"`
	Deadline string `json:"deadline"`
}

func workerInfoToJson(wi *asynq.WorkerInfo) *WorkerInfoJson {
	return &WorkerInfoJson{
		TaskID:   wi.TaskID,
		TaskType: wi.TaskType,
		Queue:    wi.Queue,
		Started:  wi.Started.UTC().Format(time.RFC3339),
		Deadline: wi.Deadline.UTC().Format(time.RFC3339),
	}
}

type ServerInfoJson struct {
	ID             string            `json:"id"`
	Host           string            `json:"host"`
	PID            int               `json:"pid"`
	Concurrency    int               `json:"concurrency"`
	Queues         map[string]int    `json:"queues"`
	StrictPriority bool              `json:"strict_priority"`
	Started        string            `json:"started"`
	Status         string            `json:"status"`
	ActiveWorkers  []*WorkerInfoJson `json:"active_workers"`
}

func serverInfoToJson(si *asynq.ServerInfo) *ServerInfoJson {
	workers := make([]*WorkerInfoJson, 0, len(si.ActiveWorkers))
	for _, w := range si.ActiveWorkers {
		workers = append(workers, workerInfoToJson(w))
	}
	return &ServerInfoJson{
		ID:             si.ID,
		Host:           si.Host,
		PID:            si.PID,
		Concurrency:    si.Concurrency,
		Queues:         si.Queues,
		StrictPriority: si.StrictPriority,
		Started:        si.Started.UTC().Format(time.RFC3339),
		Status:         si.Status,
		ActiveWorkers:  workers,
	}
}

type SchedulerEntryJson struct {
	ID       string `json:"id"`
	Spec     string `json:"spec"`
	TaskType string `json:"task_type"`
	Next     string `json:"next"`
	Prev     string `json:"prev,omitempty"`
}

func schedulerEntryToJson(se *asynq.SchedulerEntry) *SchedulerEntryJson {
	j := &SchedulerEntryJson{
		ID:       se.ID,
		Spec:     se.Spec,
		TaskType: se.Task.Type(),
		Next:     se.Next.UTC().Format(time.RFC3339),
	}
	if !se.Prev.IsZero() {
		j.Prev = se.Prev.UTC().Format(time.RFC3339)
	}
	return j
}

type BulkActionResponseJson struct {
	AffectedCount int `json:"affected_count"`
}

type ListQueuesResponseJson struct {
	Items []*QueueInfoJson `json:"items"`
}

type ListMonitoringTasksResponseJson struct {
	Items  []*MonitoringTaskInfoJson `json:"items"`
	Cursor string                    `json:"cursor,omitempty"`
}

type ListServersResponseJson struct {
	Items []*ServerInfoJson `json:"items"`
}

type ListSchedulerEntriesResponseJson struct {
	Items []*SchedulerEntryJson `json:"items"`
}

type ListQueueHistoryResponseJson struct {
	Items []*DailyStatsJson `json:"items"`
}

type taskListCursor struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Queue    string `json:"queue"`
	State    string `json:"state"`
}

// getTaskCountForState returns the total count of tasks for a given state from QueueInfo
func getTaskCountForState(qi *asynq.QueueInfo, state string) int {
	switch state {
	case "pending":
		return qi.Pending
	case "active":
		return qi.Active
	case "scheduled":
		return qi.Scheduled
	case "retry":
		return qi.Retry
	case "archived":
		return qi.Archived
	case "completed":
		return qi.Completed
	default:
		return 0
	}
}

// Handlers

func (r *TaskMonitoringRoutes) listQueues(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queues, err := r.inspector.Queues()
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list queues", httperr.WithInternalErr(err)))
		return
	}

	items := make([]*QueueInfoJson, 0, len(queues))
	for _, q := range queues {
		qi, err := r.inspector.GetQueueInfo(q)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerErrorf("failed to get queue info for %s", q))
			return
		}
		items = append(items, queueInfoToJson(qi))
	}

	gctx.PureJSON(http.StatusOK, ListQueuesResponseJson{Items: items})
}

func (r *TaskMonitoringRoutes) getQueueInfo(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")
	if queue == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("queue is required"))
		return
	}

	qi, err := r.inspector.GetQueueInfo(queue)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to get queue info", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, queueInfoToJson(qi))
}

func (r *TaskMonitoringRoutes) getQueueHistory(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")
	if queue == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("queue is required"))
		return
	}

	days := 30
	if daysStr := gctx.Query("days"); daysStr != "" {
		d, err := strconv.Atoi(daysStr)
		if err != nil || d < 1 {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid days parameter"))
			return
		}
		days = d
	}

	stats, err := r.inspector.History(queue, days)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to get queue history", httperr.WithInternalErr(err)))
		return
	}

	result := make([]*DailyStatsJson, 0, len(stats))
	for _, s := range stats {
		result = append(result, dailyStatsToJson(s))
	}

	gctx.PureJSON(http.StatusOK, ListQueueHistoryResponseJson{Items: result})
}

func (r *TaskMonitoringRoutes) listTasksByState(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")
	state := gctx.Param("state")

	if queue == "" || state == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("queue and state are required"))
		return
	}

	switch state {
	case "pending", "active", "scheduled", "retry", "archived", "completed":
		// valid
	default:
		apgin.WriteError(gctx, nil, httperr.BadRequestf("invalid state: %s", state))
		return
	}

	page := 1
	pageSize := 30
	cursorStr := gctx.Query("cursor")

	if cursorStr != "" {
		parsed, err := pagination.ParseCursor[taskListCursor](ctx, r.cursorEncryptor, cursorStr)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid cursor"))
			return
		}
		page = parsed.Page
		pageSize = parsed.PageSize
		queue = parsed.Queue
		state = parsed.State
	} else {
		if l := gctx.Query("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				pageSize = v
			}
		}
	}

	opts := []asynq.ListOption{asynq.Page(page), asynq.PageSize(pageSize)}

	var tasks []*asynq.TaskInfo
	var err error

	switch state {
	case "pending":
		tasks, err = r.inspector.ListPendingTasks(queue, opts...)
	case "active":
		tasks, err = r.inspector.ListActiveTasks(queue, opts...)
	case "scheduled":
		tasks, err = r.inspector.ListScheduledTasks(queue, opts...)
	case "retry":
		tasks, err = r.inspector.ListRetryTasks(queue, opts...)
	case "archived":
		tasks, err = r.inspector.ListArchivedTasks(queue, opts...)
	case "completed":
		tasks, err = r.inspector.ListCompletedTasks(queue, opts...)
	}

	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list tasks", httperr.WithInternalErr(err)))
		return
	}

	items := make([]*MonitoringTaskInfoJson, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, taskInfoToJson(t))
	}

	// Determine if there are more pages by checking total count from queue info
	var cursor string
	qi, qiErr := r.inspector.GetQueueInfo(queue)
	if qiErr == nil {
		total := getTaskCountForState(qi, state)
		if page*pageSize < total {
			nextCursor := taskListCursor{
				Page:     page + 1,
				PageSize: pageSize,
				Queue:    queue,
				State:    state,
			}
			cursor, _ = pagination.MakeCursor(ctx, r.cursorEncryptor, &nextCursor)
		}
	}

	gctx.PureJSON(http.StatusOK, ListMonitoringTasksResponseJson{
		Items:  items,
		Cursor: cursor,
	})
}

func (r *TaskMonitoringRoutes) getTask(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")
	taskId := gctx.Param("task_id")

	if queue == "" || taskId == "" {
		apgin.WriteError(gctx, nil, httperr.BadRequest("queue and task_id are required"))
		return
	}

	ti, err := r.inspector.GetTaskInfo(queue, taskId)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to get task info", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, taskInfoToJson(ti))
}

func (r *TaskMonitoringRoutes) listServers(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	servers, err := r.inspector.Servers()
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list servers", httperr.WithInternalErr(err)))
		return
	}

	items := make([]*ServerInfoJson, 0, len(servers))
	for _, s := range servers {
		items = append(items, serverInfoToJson(s))
	}

	gctx.PureJSON(http.StatusOK, ListServersResponseJson{Items: items})
}

func (r *TaskMonitoringRoutes) listSchedulerEntries(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	entries, err := r.inspector.SchedulerEntries()
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to list scheduler entries", httperr.WithInternalErr(err)))
		return
	}

	items := make([]*SchedulerEntryJson, 0, len(entries))
	for _, e := range entries {
		items = append(items, schedulerEntryToJson(e))
	}

	gctx.PureJSON(http.StatusOK, ListSchedulerEntriesResponseJson{Items: items})
}

func (r *TaskMonitoringRoutes) runTask(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")
	taskId := gctx.Param("task_id")

	if err := r.inspector.RunTask(queue, taskId); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to run task", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *TaskMonitoringRoutes) archiveTask(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")
	taskId := gctx.Param("task_id")

	if err := r.inspector.ArchiveTask(queue, taskId); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to archive task", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *TaskMonitoringRoutes) cancelTask(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	taskId := gctx.Param("task_id")

	if err := r.inspector.CancelProcessing(taskId); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to cancel task", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *TaskMonitoringRoutes) deleteTask(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")
	taskId := gctx.Param("task_id")

	if err := r.inspector.DeleteTask(queue, taskId); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to delete task", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *TaskMonitoringRoutes) pauseQueue(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")

	if err := r.inspector.PauseQueue(queue); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to pause queue", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *TaskMonitoringRoutes) unpauseQueue(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")

	if err := r.inspector.UnpauseQueue(queue); err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to unpause queue", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
}

func (r *TaskMonitoringRoutes) runAllArchivedTasks(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")

	count, err := r.inspector.RunAllArchivedTasks(queue)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to run all archived tasks", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, &BulkActionResponseJson{AffectedCount: count})
}

func (r *TaskMonitoringRoutes) runAllRetryTasks(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")

	count, err := r.inspector.RunAllRetryTasks(queue)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to run all retry tasks", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, &BulkActionResponseJson{AffectedCount: count})
}

func (r *TaskMonitoringRoutes) deleteAllArchivedTasks(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")

	count, err := r.inspector.DeleteAllArchivedTasks(queue)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to delete all archived tasks", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, &BulkActionResponseJson{AffectedCount: count})
}

func (r *TaskMonitoringRoutes) deleteAllCompletedTasks(gctx *gin.Context) {
	val := auth.MustGetValidatorFromGinContext(gctx)
	val.MarkValidated()

	queue := gctx.Param("queue")

	count, err := r.inspector.DeleteAllCompletedTasks(queue)
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.InternalServerErrorMsg("failed to delete all completed tasks", httperr.WithInternalErr(err)))
		return
	}

	gctx.PureJSON(http.StatusOK, &BulkActionResponseJson{AffectedCount: count})
}

// Register registers all task monitoring routes
func (r *TaskMonitoringRoutes) Register(g gin.IRouter) {
	// Read-only list endpoints
	g.GET(
		"/task-monitoring/queues",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("list").
			Build(),
		r.listQueues,
	)
	g.GET(
		"/task-monitoring/queues/:queue",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("get").
			Build(),
		r.getQueueInfo,
	)
	g.GET(
		"/task-monitoring/queues/:queue/history",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("get").
			Build(),
		r.getQueueHistory,
	)
	g.GET(
		"/task-monitoring/queues/:queue/tasks/:state",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("list").
			Build(),
		r.listTasksByState,
	)
	g.GET(
		"/task-monitoring/queues/:queue/tasks/:state/:task_id",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("get").
			Build(),
		r.getTask,
	)
	g.GET(
		"/task-monitoring/servers",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("list").
			Build(),
		r.listServers,
	)
	g.GET(
		"/task-monitoring/scheduler-entries",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("list").
			Build(),
		r.listSchedulerEntries,
	)

	// Mutating manage endpoints
	g.POST(
		"/task-monitoring/queues/:queue/tasks/:task_id/_run",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.runTask,
	)
	g.POST(
		"/task-monitoring/queues/:queue/tasks/:task_id/_archive",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.archiveTask,
	)
	g.POST(
		"/task-monitoring/queues/:queue/tasks/:task_id/_cancel",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.cancelTask,
	)
	g.DELETE(
		"/task-monitoring/queues/:queue/tasks/:task_id",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.deleteTask,
	)
	g.POST(
		"/task-monitoring/queues/:queue/_pause",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.pauseQueue,
	)
	g.POST(
		"/task-monitoring/queues/:queue/_unpause",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.unpauseQueue,
	)
	g.POST(
		"/task-monitoring/queues/:queue/archived/_run-all",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.runAllArchivedTasks,
	)
	g.POST(
		"/task-monitoring/queues/:queue/retry/_run-all",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.runAllRetryTasks,
	)
	g.DELETE(
		"/task-monitoring/queues/:queue/archived",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.deleteAllArchivedTasks,
	)
	g.DELETE(
		"/task-monitoring/queues/:queue/completed",
		r.auth.NewRequiredBuilder().
			ForResource("task_monitoring").
			ForVerb("manage").
			Build(),
		r.deleteAllCompletedTasks,
	)
}
