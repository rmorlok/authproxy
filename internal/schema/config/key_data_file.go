package config

import (
	"context"
	"fmt"
	"os"

	"github.com/mitchellh/go-homedir"
)

type KeyDataFile struct {
	Path string `json:"path" yaml:"path"`
}

func (kf *KeyDataFile) resolvePath() (string, error) {
	path := kf.Path
	if _, err := os.Stat(path); os.IsNotExist(err) {
		expanded, err2 := homedir.Expand(kf.Path)
		if err2 != nil {
			return "", err
		}

		if _, err := os.Stat(expanded); err != nil {
			return "", fmt.Errorf("key file '%s' does not exist", kf.Path)
		}
		path = expanded
	}
	return path, nil
}

func (kf *KeyDataFile) GetCurrentVersion(ctx context.Context) (KeyVersionInfo, error) {
	path, err := kf.resolvePath()
	if err != nil {
		return KeyVersionInfo{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return KeyVersionInfo{}, err
	}

	return KeyVersionInfo{
		Provider:        ProviderTypeFile,
		ProviderID:      kf.Path,
		ProviderVersion: DataHash(data),
		Data:            data,
		IsCurrent:       true,
	}, nil
}

func (kf *KeyDataFile) GetVersion(ctx context.Context, version string) (KeyVersionInfo, error) {
	return getVersionFromList(ctx, kf, version)
}

func (kf *KeyDataFile) ListVersions(ctx context.Context) ([]KeyVersionInfo, error) {
	v, err := kf.GetCurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return []KeyVersionInfo{v}, nil
}

func (kf *KeyDataFile) GetProviderType() ProviderType {
	return ProviderTypeFile
}

var _ KeyDataType = (*KeyDataFile)(nil)
