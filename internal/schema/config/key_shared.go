package config

type KeyShared struct {
	SharedKey *KeyData `json:"shared_key" yaml:"shared_key"`
}

func (ks *KeyShared) CanSign() bool {
	return true
}

func (ks *KeyShared) CanVerifySignature() bool {
	return true
}

var _ KeyType = (*KeyShared)(nil)
