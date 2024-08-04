package config

type AuthApiKey struct {
	Type AuthType `json:"type" yaml:"type"`
}

func (a *AuthApiKey) GetType() AuthType {
	return AuthTypeAPIKey
}
