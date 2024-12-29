package config

import (
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"os"
)

type StringValueEnvVar struct {
	EnvVar string `json:"env_var" yaml:"env_var"`
}

func (kev *StringValueEnvVar) HasValue(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return present && len(val) > 0
}

func (kev *StringValueEnvVar) GetValue(ctx context.Context) (string, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		return "", errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}
	return val, nil
}
