package core

import apworkflows "github.com/rmorlok/authproxy/internal/workflows"

func (s *service) RegisterWorkflows(worker *apworkflows.Worker) error {
	registerFns := []func(workflowRegistrar) error{
		s.registerDisconnectConnectionWorkflow,
		s.registerDisconnectConnectorConnectionsWorkflow,
	}

	for _, registerFn := range registerFns {
		if err := registerFn(worker); err != nil {
			return err
		}
	}

	return nil
}
