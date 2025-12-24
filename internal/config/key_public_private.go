package config

type KeyPublicPrivate struct {
	PublicKey  *KeyData `json:"public_key" yaml:"public_key"`
	PrivateKey *KeyData `json:"private_key" yaml:"private_key"`
}

func (kpp *KeyPublicPrivate) CanSign() bool {
	return kpp.PrivateKey != nil
}

func (kpp *KeyPublicPrivate) CanVerifySignature() bool {
	return kpp.PublicKey != nil
}
