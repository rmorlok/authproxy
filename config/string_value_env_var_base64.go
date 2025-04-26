package config

import (
	"context"
	"encoding/base64"
	"github.com/pkg/errors"
	"os"
)

type StringValueEnvVarBase64 struct {
	EnvVar string `json:"env_var_base64" yaml:"env_var_base64"`
}

func (kev *StringValueEnvVarBase64) HasValue(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return present && len(val) > 0
}

func (kev *StringValueEnvVarBase64) GetValue(ctx context.Context) (string, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		return "", errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return "", errors.Wrapf(err, "environment variable '%s' value is not valid base64", kev.EnvVar)
	}

	return string(decodedBytes), nil
}
