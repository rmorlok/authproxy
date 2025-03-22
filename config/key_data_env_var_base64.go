package config

import (
	"encoding/base64"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/context"
	"os"
)

type KeyDataEnvBase64Var struct {
	EnvVar string `json:"env_var_base64" yaml:"env_var_base64"`
}

func (kev *KeyDataEnvBase64Var) HasData(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return present && len(val) > 0
}

func (kev *KeyDataEnvBase64Var) GetData(ctx context.Context) ([]byte, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		return nil, errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, errors.Wrapf(err, "environment variable '%s' value is not valid base64", kev.EnvVar)
	}

	return decodedBytes, nil
}
