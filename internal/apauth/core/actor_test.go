package core

import (
	"crypto/sha1" //nolint
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestActor_HashID(t *testing.T) {
	t.Parallel()
	tbl := []struct {
		id   string
		hash string
	}{
		{"myid", "6e34471f84557e1713012d64a7477c71bfdac631"},
		{"", "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{"blah blah", "135a1e01bae742c4a576b20fd41a683f6483ca43"},
		{"da39a3ee5e6b4b0d3255bfef95601890afd80709", "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
	}

	for i, tt := range tbl {
		hh := sha1.New()
		assert.Equal(t, tt.hash, HashID(hh, tt.id), "case #%d", i)
	}
}

type mockBadHasher struct{}

func (m *mockBadHasher) Write(p []byte) (n int, err error) { return 0, errors.New("err") }
func (m *mockBadHasher) Sum(b []byte) []byte               { return nil }
func (m *mockBadHasher) Reset()                            {}
func (m *mockBadHasher) Size() int                         { return 0 }
func (m *mockBadHasher) BlockSize() int                    { return 0 }

func TestActor(t *testing.T) {
	t.Parallel()
	t.Run("HashIDWithCRC", func(t *testing.T) {
		tbl := []struct {
			id   string
			hash string
		}{
			{"myid", "e337514486e387ed"},
			{"", "914cd8098b8a2128"},
			{"blah blah", "a9d6c06bfd811649"},
			{"a9d6c06bfd811649", "a9d6c06bfd811649"},
		}

		for i, tt := range tbl {
			hh := &mockBadHasher{}
			assert.Equal(t, tt.hash, HashID(hh, tt.id), "case #%d", i)
		}
	})
	t.Run("IsAdmin", func(t *testing.T) {
		u := Actor{}
		assert.False(t, u.IsAdmin())
		u.Admin = true
		assert.True(t, u.IsAdmin())
		u.Admin = false
		assert.False(t, u.IsAdmin())

		var nila *Actor
		assert.False(t, nila.IsAdmin())
	})
	t.Run("IsSuperAdmin", func(t *testing.T) {
		u := Actor{}
		assert.False(t, u.IsSuperAdmin())
		u.SuperAdmin = true
		assert.True(t, u.IsSuperAdmin())
		u.SuperAdmin = false
		assert.False(t, u.IsSuperAdmin())

		var nila *Actor
		assert.False(t, nila.IsSuperAdmin())
	})
	t.Run("IsNormalActor", func(t *testing.T) {
		u := Actor{}
		assert.True(t, u.IsNormalActor())
		u.SuperAdmin = true
		assert.False(t, u.IsNormalActor())
		u.SuperAdmin = false
		u.Admin = true
		assert.False(t, u.IsNormalActor())
		u.SuperAdmin = false
		u.Admin = false
		assert.True(t, u.IsNormalActor())

		var nila *Actor
		assert.True(t, nila.IsNormalActor())
	})
}
