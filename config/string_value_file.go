package config

import (
	"context"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"os"
)

type StringValueFile struct {
	Path string `json:"path" yaml:"path"`
}

func (kf *StringValueFile) HasValue(ctx context.Context) bool {
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

func (kf *StringValueFile) GetValue(ctx context.Context) (string, error) {
	path := kf.Path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// attempt home path expansion
		var err2 error
		path, err2 = homedir.Expand(kf.Path)
		if err2 != nil {
			return "", err
		}

		if _, err := os.Stat(path); err != nil {
			return "", errors.Errorf("key file '%s' does not exist", kf.Path)
		}
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Read the file contents
	return string(bytes), nil
}
