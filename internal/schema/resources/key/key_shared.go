package key

type KeyShared struct {
	SharedKey *KeyData `json:"sharedKey" yaml:"sharedKey"`
}

func (ks *KeyShared) CanSign() bool {
	return true
}

func (ks *KeyShared) CanVerifySignature() bool {
	return true
}

var _ SigningKeyType = (*KeyShared)(nil)
