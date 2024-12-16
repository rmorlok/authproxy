package config

import (
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"os"
)

type KeyDataFile struct {
	Path string `json:"path" yaml:"path"`
}

func (kf *KeyDataFile) HasData(ctx context.Context) bool {
	if _, err := os.Stat(kf.Path); os.IsNotExist(err) {
		return false
	}

	return true
}

func (kf *KeyDataFile) GetData(ctx context.Context) ([]byte, error) {
	if _, err := os.Stat(kf.Path); os.IsNotExist(err) {
		return nil, errors.Errorf("key file '%s' does not exist", kf.Path)
	}

	// Read the file contents
	return os.ReadFile(kf.Path)
}
