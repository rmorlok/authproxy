package config

import (
	"context"
	"os"

	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
)

type KeyDataFile struct {
	Path string `json:"path" yaml:"path"`
}

func (kf *KeyDataFile) HasData(ctx context.Context) bool {
	if _, err := os.Stat(kf.Path); err != nil {
		// attempt home path expansion
		path, err := homedir.Expand(kf.Path)
		if err != nil {
			return false
		}

		if _, err := os.Stat(path); err != nil {
			return false
		}
	}

	return true
}

func (kf *KeyDataFile) GetData(ctx context.Context) ([]byte, error) {
	path := kf.Path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// attempt home path expansion
		var err2 error
		path, err2 = homedir.Expand(kf.Path)
		if err2 != nil {
			return nil, err
		}

		if _, err := os.Stat(path); err != nil {
			return nil, errors.Errorf("key file '%s' does not exist", kf.Path)
		}
	}

	// Read the file contents
	return os.ReadFile(path)
}

var _ KeyDataType = (*KeyDataFile)(nil)
