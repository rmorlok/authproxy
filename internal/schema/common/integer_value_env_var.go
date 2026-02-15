package common

import (
	"context"
	"os"
	"strconv"

	"github.com/pkg/errors"
)

type IntegerValueEnvVar struct {
	EnvVar  string `json:"env_var" yaml:"env_var"`
	Default *int64 `json:"default,omitempty" yaml:"default,omitempty"`
}

func (kev *IntegerValueEnvVar) HasValue(ctx context.Context) bool {
	val, present := os.LookupEnv(kev.EnvVar)
	return present && len(val) > 0
}

func (kev *IntegerValueEnvVar) GetValue(ctx context.Context) (int64, error) {
	strVal, present := os.LookupEnv(kev.EnvVar)

	if !present || len(strVal) == 0 {
		if kev.Default != nil {
			return *kev.Default, nil
		}

		return 0, errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}

	val, err := strconv.ParseInt(strVal, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse environment variable '%s' with value '%s' as int64", kev.EnvVar, strVal)
	}

	return val, nil
}

func (kev *IntegerValueEnvVar) GetUint64Value(ctx context.Context) (uint64, error) {
	val, err := kev.GetValue(ctx)
	return uint64(val), err
}

func (kb *IntegerValueEnvVar) Clone() IntegerValueType {
	if kb == nil {
		return nil
	}

	clone := *kb
	return &clone
}

var _ IntegerValueType = (*IntegerValueEnvVar)(nil)
