package config

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
)

type KeyDataEnvBase64Var struct {
	EnvVar string `json:"env_var_base64" yaml:"env_var_base64"`
}

func (kev *KeyDataEnvBase64Var) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	val, present := os.LookupEnv(kev.EnvVar)
	if !present || len(val) == 0 {
		return KeyVersionInfo{}, fmt.Errorf("environment variable '%s' does not have value", kev.EnvVar)
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return KeyVersionInfo{}, fmt.Errorf("environment variable '%s' value is not valid base64: %w", kev.EnvVar, err)
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeEnvVarBase64,
		ProviderID:      kev.EnvVar,
		ProviderVersion: DataHash(decodedBytes),
		Data:            decodedBytes,
		IsCurrent:       true,
	}, nil
}

func (kev *KeyDataEnvBase64Var) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	return getVersionFromList(ctx, kev, version)
}

func (kev *KeyDataEnvBase64Var) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kev.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kev *KeyDataEnvBase64Var) GetProviderType() ProviderType {
	return ProviderTypeEnvVarBase64
}

var _ KeyDataType = (*KeyDataEnvBase64Var)(nil)
