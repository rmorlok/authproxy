package config

type ErrorPages struct {
	Unauthorized string `json:"unauthorized" yaml:"unauthorized"`
	Fallback     string `json:"fallback" yaml:"fallback"`
}

func (e *ErrorPages) GetUnauthorized() string {
	if e.Unauthorized != "" {
		return e.Unauthorized
	}

	return e.Fallback
}
