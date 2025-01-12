package config

import "fmt"

type ApiHost struct {
	Port uint64 `json:"port" yaml:"port"`
}

func (a *ApiHost) IsHttps() bool {
	return false
}

func (a *ApiHost) GetBaseUrl() string {
	return fmt.Sprintf("http://localhost:%04d", a.Port)
}
