package connectors

import (
	"github.com/hibiken/asynq"
)

func (s *service) RegisterTasks(mux *asynq.ServeMux) {
	mux.HandleFunc(taskTypeMigrateConnectionsBetweenConnectorVersions, s.migrateConnectionsBetweenConnectorVersions)
}

func (s *service) GetCronTasks() []*asynq.PeriodicTaskConfig {
	return []*asynq.PeriodicTaskConfig{}
}
