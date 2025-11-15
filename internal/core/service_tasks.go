package core

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeMigrateConnectionsBetweenConnectorVersions, s.migrateConnectionsBetweenConnectorVersions)
	mux.HandleFunc(taskTypeDisconnectConnection, s.disconnectConnection)
	mux.HandleFunc(taskTypeProbe, s.runProbeForConnection)
}

func (s *service) GetCronTasks() []*asynq.PeriodicTaskConfig {
	s.logger.Info("refreshing core service periodic tasks")
	start := time.Now()
	defer func() {
		s.logger.Info("refreshing core service tasks periodic completed", "duration", time.Since(start))
	}()

	periodTasks := []*asynq.PeriodicTaskConfig{}
	err := s.db.EnumerateConnections(
		context.Background(),
		database.DeletedHandlingExclude,
		[]database.ConnectionState{
			database.ConnectionStateCreated,
			database.ConnectionStateReady,
		},
		func(conns []*database.Connection, lastPage bool) (stop bool, err error) {
			for _, dbConn := range conns {
				logger := aplog.NewBuilder(s.logger).
					WithConnectionId(dbConn.ID).
					Build()
				c, err := s.getConnectionForDb(context.Background(), dbConn)
				if err != nil {
					s.logger.Error("failed to get connection to scheduled periodic tasks", "error", err)
					continue
				}

				for _, probe := range c.GetProbes() {
					if probe.IsPeriodic() {
						logger.Debug("adding periodic probe task", "probe_id", probe.GetId())
						t, err := newProbeTask(c.ID, probe.GetId())
						if err != nil {
							logger.Error("failed to create probe task", "error", err, "probe_id", probe.GetId())
							continue
						}

						periodTasks = append(periodTasks, &asynq.PeriodicTaskConfig{
							Task:     t,
							Cronspec: probe.GetScheduleString(),
						})
						logger.Debug("added periodic probe task", "probe", probe.GetId(), "schedule", probe.GetScheduleString())
					}
				}
			}

			return false, nil
		},
	)

	if err != nil {
		s.logger.Error("failed to enumerate connections for periodic tasks", "error", err)
	}

	return periodTasks
}
