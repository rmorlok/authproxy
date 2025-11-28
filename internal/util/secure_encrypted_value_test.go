package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecureDecryptedJsonValue(t *testing.T) {
	t.Parallel()

	type Foo struct {
		Val string
	}

	t.Run("successfully roundtrips", func(t *testing.T) {
		key := MustGenerateSecureRandomKey(16)
		val := Foo{
			Val: "Bar",
		}
		encrypted, err := SecureEncryptedJsonValue(key, val)
		assert.NoError(t, err)
		assert.NotContains(t, string(encrypted), string("Bar"))

		newVal, err := SecureDecryptedJsonValue[Foo](key, encrypted)
		assert.NoError(t, err)
		assert.Equal(t, val, *newVal)
	})

	t.Run("fails with different keys", func(t *testing.T) {
		key := MustGenerateSecureRandomKey(16)
		key2 := MustGenerateSecureRandomKey(16)
		val := Foo{
			Val: "Bar",
		}
		encrypted, err := SecureEncryptedJsonValue(key, val)
		assert.NoError(t, err)

		_, err = SecureDecryptedJsonValue[Foo](key2, encrypted)
		assert.Error(t, err)
	})

	t.Run("fails if data changes", func(t *testing.T) {
		key := MustGenerateSecureRandomKey(16)
		val := Foo{
			Val: "Bar",
		}
		encrypted, err := SecureEncryptedJsonValue(key, val)
		assert.NoError(t, err)

		encryptedBytes := []byte(encrypted)
		encryptedBytes[5] = 'x'

		_, err = SecureDecryptedJsonValue[Foo](key, string(encryptedBytes))
		assert.Error(t, err)
	})
}
