package test_utils

import (
	"os"
	"path/filepath"
)

// TestDataPath returns an absolute path from the test_data folder in this project
func TestDataPath(relativePath string) string {
	return filepath.Join(MustGetProjectRoot(), "test_data", relativePath)
}

// MustGetProjectRoot returns an absolute path to the root of this golang project
func MustGetProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	for {
		// Check if the go.mod file exists (which usually indicates the project root)
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		// Move one level up
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			// Reached the root of the filesystem without finding go.mod
			panic("could not find go.mod to establish project root")
		}
		dir = parentDir
	}
}
