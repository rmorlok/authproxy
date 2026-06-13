package core

import (
	"github.com/cschleiden/go-workflows/registry"
	wflib "github.com/cschleiden/go-workflows/workflow"
	apworkflows "github.com/rmorlok/authproxy/internal/workflows"
)

type workflowRegistrar interface {
	RegisterWorkflow(workflow wflib.Workflow, opts ...registry.RegisterOption) error
	RegisterActivity(activity wflib.Activity, opts ...registry.RegisterOption) error
}

func (s *service) RegisterWorkflows(worker *apworkflows.Worker) error {
	registerFns := []func(workflowRegistrar) error{
		s.registerDisconnectConnectionWorkflow,
		s.registerDisconnectConnectorConnectionsWorkflow,
		s.registerArchiveConnectorWorkflow,
	}

	for _, registerFn := range registerFns {
		if err := registerFn(worker); err != nil {
			return err
		}
	}

	return nil
}
