package config

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestTasksGetWorkflowRetention(t *testing.T) {
	t.Run("defaults when the tasks block is absent", func(t *testing.T) {
		var tasks *Tasks
		assert.Equal(t, DefaultWorkflowRetention, tasks.GetWorkflowRetention())
	})

	t.Run("defaults when the field is unset", func(t *testing.T) {
		assert.Equal(t, DefaultWorkflowRetention, (&Tasks{}).GetWorkflowRetention())
	})

	t.Run("uses the configured value", func(t *testing.T) {
		tasks := &Tasks{WorkflowRetention: common.HumanDurationFor("7d")}
		assert.Equal(t, 7*24*time.Hour, tasks.GetWorkflowRetention())
	})

	t.Run("preserves zero so cleanup can be disabled", func(t *testing.T) {
		tasks := &Tasks{WorkflowRetention: common.HumanDurationFor("0s")}
		assert.Zero(t, tasks.GetWorkflowRetention())
	})
}

func TestTasksWorkflowRetentionYaml(t *testing.T) {
	// The documented hour-based form must load from a YAML config file.
	var tasks Tasks
	assert.NoError(t, yaml.Unmarshal([]byte("workflowRetention: 720h"), &tasks))
	assert.Equal(t, DefaultWorkflowRetention, tasks.GetWorkflowRetention())
}
