package connectors

type AuthApiKey struct {
	Type AuthType `json:"type" yaml:"type"`
}

func (a *AuthApiKey) GetType() AuthType {
	return AuthTypeAPIKey
}

func (a *AuthApiKey) Clone() AuthImpl {
	if a == nil {
		return nil
	}

	clone := *a
	return &clone
}

var _ AuthImpl = (*AuthApiKey)(nil)
