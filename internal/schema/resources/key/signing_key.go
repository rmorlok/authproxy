package key

// SigningKeyType is implemented by the supported signing-key shapes.
// Signing keys are configuration values, not managed Key resources.
type SigningKeyType interface {
	// CanSign checks if the key can sign requests (either private key is present or shared key)
	CanSign() bool
	// CanVerifySignature checks if the key can be used to verify the signature of something (public key is present or shared key)
	CanVerifySignature() bool
}

// SigningKey is public/private or shared signing material used by AuthProxy
// configuration. The managed API resource is Key.
type SigningKey struct {
	InnerVal SigningKeyType `json:"-" yaml:"-"`
}

func (k *SigningKey) CanSign() bool {
	return k.InnerVal.CanSign()
}

func (k *SigningKey) CanVerifySignature() bool {
	return k.InnerVal.CanVerifySignature()
}

var _ SigningKeyType = (*SigningKey)(nil)
