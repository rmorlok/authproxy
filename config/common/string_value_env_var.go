package common

import (
	"context"
	"github.com/pkg/errors"
	"os"
)

type StringValueEnvVar struct {
	EnvVar  string  `json:"env_var" yaml:"env_var"`
	Default *string `json:"default,omitempty" yaml:"default,omitempty"`
}

func (kev *StringValueEnvVar) HasValue(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return present && len(val) > 0
}

func (kev *StringValueEnvVar) GetValue(ctx context.Context) (string, error) {
	val, present := os.LookupEnv(kev.EnvVar)

	if !present || len(val) == 0 {
		if kev.Default != nil {
			return *kev.Default, nil
		}

		return "", errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}
	return val, nil
}