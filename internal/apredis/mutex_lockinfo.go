package apredis

import (
	"encoding/json"
	"os"
)

// lockInfo is a struct of data that can be appended to a lock as metadata to provide debugging
// information about which host obtained the lock.
type lockInfo struct {
	Hostname    string `json:"hostname,omitempty"`
	ProcessID   int    `json:"pid,omitempty"`
	GoRoutineID string `json:"goroutine_id,omitempty"`
	ThreadID    string `json:"thread_id,omitempty"`
	Environment string `json:"environment,omitempty"` // e.g., "prod", "staging"
	Region      string `json:"region,omitempty"`      // e.g., "us-west-2"
	Container   string `json:"container,omitempty"`   // For containerized environments
	Pod         string `json:"pod,omitempty"`         // For Kubernetes
}

// Helper function to get container ID (if running in Docker)
func getContainerID() string {
	content, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return ""
	}
	// Parse container ID from cgroup file
	// This is a simplified example
	return string(content)
}

// Enhanced version with more information
func generateDetailedLockValue() string {
	hostname, _ := os.Hostname()

	info := lockInfo{
		Hostname:    hostname,
		ProcessID:   os.Getpid(),
		Environment: os.Getenv("ENV"),
		Region:      os.Getenv("AWS_REGION"), // if using AWS
		Container:   getContainerID(),
		Pod:         os.Getenv("POD_NAME"), // if using Kubernetes
	}

	value, err := json.Marshal(info)
	if err != nil {
		return ""
	}

	return string(value)
}
