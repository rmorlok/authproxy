package common

import (
	"context"
	"encoding/base64"
	"os"

	"github.com/pkg/errors"
)

type StringValueEnvVarBase64 struct {
	EnvVar  string  `json:"env_var_base64" yaml:"env_var_base64"`
	Default *string `json:"default,omitempty" yaml:"default,omitempty"`
}

func (kev *StringValueEnvVarBase64) HasValue(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return (present && len(val) > 0) || (kev.Default != nil && len(*kev.Default) > 0)
}

func (kev *StringValueEnvVarBase64) GetValue(ctx context.Context) (string, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		if kev.Default != nil {
			val = *kev.Default
		} else {
			return "", errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
		}
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return "", errors.Wrapf(err, "environment variable '%s' value is not valid base64", kev.EnvVar)
	}

	return string(decodedBytes), nil
}

func (kb *StringValueEnvVarBase64) Clone() StringValueType {
	if kb == nil {
		return nil
	}

	clone := *kb
	return &clone
}

var _ StringValueType = (*StringValueEnvVarBase64)(nil)
