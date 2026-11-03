package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/config"
	dbMock "github.com/rmorlok/authproxy/internal/database/mock"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

func TestCleanupExpiredTaskCheckpointsTask(t *testing.T) {
	task := asynq.NewTask(taskTypeCleanupExpiredTaskCheckpoints, nil)

	t.Run("delegates to the database", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		mockDB.EXPECT().DeleteExpiredTaskCheckpoints(gomock.Any()).Return(int64(2), nil)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.NoError(t, th.cleanupExpiredTaskCheckpoints(context.Background(), task))
	})

	t.Run("propagates DB errors so asynq retries", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := dbMock.NewMockDB(ctrl)
		dbErr := errors.New("transient")
		mockDB.EXPECT().DeleteExpiredTaskCheckpoints(gomock.Any()).Return(int64(0), dbErr)

		th := &taskHandler{db: mockDB, logger: aplog.NewNoopLogger()}
		require.ErrorIs(t, th.cleanupExpiredTaskCheckpoints(context.Background(), task), dbErr)
	})
}

func TestNewCleanupExpiredTaskCheckpointsTask(t *testing.T) {
	task := newCleanupExpiredTaskCheckpointsTask()
	require.Equal(t, taskTypeCleanupExpiredTaskCheckpoints, task.Type())
}

func TestGetCronTasksSchedulesCheckpointCleanup(t *testing.T) {
	th := &taskHandler{cfg: config.FromRoot(&sconfig.Root{}), logger: aplog.NewNoopLogger()}

	// Being scheduled is what stops abandoned checkpoints from accumulating.
	var cronspecs []string
	for _, cron := range th.GetCronTasks() {
		if cron.Task.Type() == taskTypeCleanupExpiredTaskCheckpoints {
			cronspecs = append(cronspecs, cron.Cronspec)
		}
	}
	require.Equal(t, []string{"@daily"}, cronspecs)
}
