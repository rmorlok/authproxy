package connectors

type AuthNoAuth struct {
	Type AuthType `json:"type" yaml:"type"`
}

func (a *AuthNoAuth) GetType() AuthType {
	return AuthTypeNoAuth
}

func (a *AuthNoAuth) Clone() Auth {
	if a == nil {
		return nil
	}

	clone := *a
	return &clone
}

var _ Auth = (*AuthNoAuth)(nil)
