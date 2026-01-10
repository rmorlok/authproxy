package config

// DevSettings are flags that can be set to turn auth proxy into developer mode to make it easer to test and see
// what is going on in the system. These settings should not be enabled in production.
type DevSettings struct {
	Enabled                  bool `json:"enabled" yaml:"enabled"`
	FakeEncryption           bool `json:"fake_encryption" yaml:"fake_encryption"`
	FakeEncryptionSkipBase64 bool `json:"fake_encryption_skip_base64" yaml:"fake_encryption_skip_base64"`
}

func (d *DevSettings) IsEnabled() bool {
	if d == nil {
		return false
	}

	return d.Enabled
}

func (d *DevSettings) IsFakeEncryptionEnabled() bool {
	if !d.IsEnabled() {
		return false
	}

	return d.FakeEncryption
}

func (d *DevSettings) IsFakeEncryptionSkipBase64Enabled() bool {
	if !d.IsEnabled() {
		return false
	}

	return d.FakeEncryptionSkipBase64
}
