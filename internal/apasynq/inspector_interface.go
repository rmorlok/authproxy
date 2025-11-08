package apasynq

import "github.com/hibiken/asynq"

//go:generate mockgen -source=inspector_interface.go -destination=./mock/mock_inspector_interface.go -package=mock
type Inspector interface {
	Close() error
	Queues() ([]string, error)
	Groups(queue string) ([]*asynq.GroupInfo, error)
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
	History(queue string, n int) ([]*asynq.DailyStats, error)
	DeleteQueue(queue string, force bool) error
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
	ListPendingTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListActiveTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListAggregatingTasks(queue, group string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListScheduledTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListRetryTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListArchivedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	ListCompletedTasks(queue string, opts ...asynq.ListOption) ([]*asynq.TaskInfo, error)
	DeleteAllPendingTasks(queue string) (int, error)
	DeleteAllScheduledTasks(queue string) (int, error)
	DeleteAllRetryTasks(queue string) (int, error)
	DeleteAllArchivedTasks(queue string) (int, error)
	DeleteAllCompletedTasks(queue string) (int, error)
	DeleteAllAggregatingTasks(queue, group string) (int, error)
	DeleteTask(queue, id string) error
	RunAllScheduledTasks(queue string) (int, error)
	RunAllRetryTasks(queue string) (int, error)
	RunAllArchivedTasks(queue string) (int, error)
	RunAllAggregatingTasks(queue, group string) (int, error)
	RunTask(queue, id string) error
	ArchiveAllPendingTasks(queue string) (int, error)
	ArchiveAllScheduledTasks(queue string) (int, error)
	ArchiveAllRetryTasks(queue string) (int, error)
	ArchiveAllAggregatingTasks(queue, group string) (int, error)
	ArchiveTask(queue, id string) error
	CancelProcessing(id string) error
	PauseQueue(queue string) error
	UnpauseQueue(queue string) error
	Servers() ([]*asynq.ServerInfo, error)
	ClusterKeySlot(queue string) (int64, error)
	ClusterNodes(queue string) ([]*asynq.ClusterNode, error)
	SchedulerEntries() ([]*asynq.SchedulerEntry, error)
	ListSchedulerEnqueueEvents(entryID string, opts ...asynq.ListOption) ([]*asynq.SchedulerEnqueueEvent, error)
}

var _ Inspector = (*asynq.Inspector)(nil)
