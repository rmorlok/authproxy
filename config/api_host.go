package config

type ApiHost struct {
	Port uint64 `json:"port" yaml:"port"`
}

func (a *ApiHost) IsHttps() bool {
	return false
}
