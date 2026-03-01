package apid

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	id := New(PrefixActor)
	require.True(t, id.HasPrefix(PrefixActor))
	require.Equal(t, PrefixActor, id.Prefix())
	require.False(t, id.IsNil())
	// prefix (4) + suffix (16) = 20
	require.Len(t, string(id), 4+suffixLen)

	// Two generated IDs should differ
	id2 := New(PrefixActor)
	require.NotEqual(t, id, id2)
}

func TestNewSessionPrefix(t *testing.T) {
	id := New(PrefixSession)
	require.True(t, id.HasPrefix(PrefixSession))
	// prefix (5) + suffix (16) = 21
	require.Len(t, string(id), 5+suffixLen)
}

func TestParse(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		id, err := Parse("act_7Ks9mPqR2xvN3bXY")
		require.NoError(t, err)
		require.Equal(t, ID("act_7Ks9mPqR2xvN3bXY"), id)
	})

	t.Run("empty", func(t *testing.T) {
		id, err := Parse("")
		require.NoError(t, err)
		require.Equal(t, Nil, id)
	})

	t.Run("unknown prefix", func(t *testing.T) {
		_, err := Parse("zzz_abc123")
		require.Error(t, err)
	})
}

func TestMustParse(t *testing.T) {
	require.NotPanics(t, func() {
		MustParse("act_testvalue00001")
	})
	require.Panics(t, func() {
		MustParse("bad_testvalue00001")
	})
}

func TestNil(t *testing.T) {
	var id ID
	require.True(t, id.IsNil())
	require.Equal(t, Nil, id)
	require.Equal(t, "", id.String())
}

func TestHasPrefix(t *testing.T) {
	id := MustParse("cxn_3bTfWx8yLmQz4kAB")
	require.True(t, id.HasPrefix(PrefixConnection))
	require.False(t, id.HasPrefix(PrefixActor))
}

func TestJSON(t *testing.T) {
	id := MustParse("act_7Ks9mPqR2xvN3bXY")

	data, err := json.Marshal(id)
	require.NoError(t, err)
	require.Equal(t, `"act_7Ks9mPqR2xvN3bXY"`, string(data))

	var parsed ID
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)
	require.Equal(t, id, parsed)
}

func TestJSONNil(t *testing.T) {
	data, err := json.Marshal(Nil)
	require.NoError(t, err)
	require.Equal(t, `""`, string(data))
}

func TestSQLScanAndValue(t *testing.T) {
	id := MustParse("tok_5cRvBm7nYtAs9xAB")

	val, err := id.Value()
	require.NoError(t, err)
	require.Equal(t, "tok_5cRvBm7nYtAs9xAB", val)

	var scanned ID
	err = scanned.Scan("tok_5cRvBm7nYtAs9xAB")
	require.NoError(t, err)
	require.Equal(t, id, scanned)
}

func TestSQLNilValue(t *testing.T) {
	val, err := Nil.Value()
	require.NoError(t, err)
	require.Equal(t, driver.Value(nil), val)

	var scanned ID
	err = scanned.Scan(nil)
	require.NoError(t, err)
	require.Equal(t, Nil, scanned)
}

func TestValidatePrefix(t *testing.T) {
	t.Run("nil ID returns no error", func(t *testing.T) {
		require.NoError(t, Nil.ValidatePrefix(PrefixActor))
	})
	t.Run("correct prefix returns no error", func(t *testing.T) {
		id := New(PrefixActor)
		require.NoError(t, id.ValidatePrefix(PrefixActor))
	})
	t.Run("wrong prefix returns error", func(t *testing.T) {
		id := New(PrefixConnection)
		err := id.ValidatePrefix(PrefixActor)
		require.Error(t, err)
		require.Contains(t, err.Error(), `expected prefix "act_"`)
		require.Contains(t, err.Error(), `got "cxn_"`)
	})
}

func TestScanBytes(t *testing.T) {
	var scanned ID
	err := scanned.Scan([]byte("act_7Ks9mPqR2xvN3bXY"))
	require.NoError(t, err)
	require.Equal(t, ID("act_7Ks9mPqR2xvN3bXY"), scanned)
}
