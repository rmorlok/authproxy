package config

import (
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"os"
)

type KeyDataEnvVar struct {
	EnvVar string `json:"env_var" yaml:"env_var"`
}

func (kev *KeyDataEnvVar) HasData(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return present && len(val) > 0
}

func (kev *KeyDataEnvVar) GetData(ctx context.Context) ([]byte, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		return nil, errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}
	return []byte(val), nil
}
