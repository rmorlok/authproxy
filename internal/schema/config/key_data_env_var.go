package config

import (
	"context"
	"os"

	"github.com/pkg/errors"
)

type KeyDataEnvVar struct {
	EnvVar string `json:"env_var" yaml:"env_var"`
}

func (kev *KeyDataEnvVar) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		return KeyVersionInfo{}, errors.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}
	data := []byte(val)
	return KeyVersionInfo{
		Provider:        ProviderTypeEnvVar,
		ProviderID:      kev.EnvVar,
		ProviderVersion: DataHash(data),
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (kev *KeyDataEnvVar) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kev.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kev *KeyDataEnvVar) GetProviderType() ProviderType {
	return ProviderTypeEnvVar
}

var _ KeyDataType = (*KeyDataEnvVar)(nil)
