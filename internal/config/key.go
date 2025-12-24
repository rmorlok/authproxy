package config

type KeyType interface {
	// CanSign checks if the key can sign requests (either private key is present or shared key)
	CanSign() bool
	// CanVerifySignature checks if the key can be used to verify the signature of something (public key is present or shared key)
	CanVerifySignature() bool
}

type Key struct {
	InnerVal KeyType `json:"-" yaml:"-"`
}

func (k *Key) CanSign() bool {
	return k.InnerVal.CanSign()
}

func (k *Key) CanVerifySignature() bool {
	return k.InnerVal.CanVerifySignature()
}

var _ KeyType = (*Key)(nil)
