package config

type KeyShared struct {
	SharedKey string `json:"shared_key" yaml:"shared_key"`
}

func (kpp *KeyShared) CanSign() bool {
	return true
}

func (kpp *KeyShared) CanVerifySignature() bool {
	return true
}
