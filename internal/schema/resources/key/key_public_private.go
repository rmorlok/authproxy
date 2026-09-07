package key

type KeyPublicPrivate struct {
	PublicKey  *KeyData `json:"publicKey" yaml:"publicKey"`
	PrivateKey *KeyData `json:"privateKey" yaml:"privateKey"`
}

func (kpp *KeyPublicPrivate) CanSign() bool {
	return kpp.PrivateKey != nil
}

func (kpp *KeyPublicPrivate) CanVerifySignature() bool {
	return kpp.PublicKey != nil
}

var _ SigningKeyType = (*KeyPublicPrivate)(nil)
