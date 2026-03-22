package encfield

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptedField_IsZero(t *testing.T) {
	assert.True(t, EncryptedField{}.IsZero())
	assert.False(t, EncryptedField{ID: "ekv_abc", Data: "data"}.IsZero())
	assert.False(t, EncryptedField{ID: "ekv_abc"}.IsZero())
	assert.False(t, EncryptedField{Data: "data"}.IsZero())
}

func TestEncryptedField_IsEncryptedWithKey(t *testing.T) {
	ef := EncryptedField{ID: "ekv_abc", Data: "data"}
	assert.True(t, ef.IsEncryptedWithKey("ekv_abc"))
	assert.False(t, ef.IsEncryptedWithKey("ekv_xyz"))
}

func TestEncryptedField_Value_Zero(t *testing.T) {
	ef := EncryptedField{}
	v, err := ef.Value()
	require.NoError(t, err)
	assert.Nil(t, v)
}

func TestEncryptedField_Value_NonZero(t *testing.T) {
	ef := EncryptedField{ID: "ekv_abc", Data: "c29tZWRhdGE="}
	v, err := ef.Value()
	require.NoError(t, err)
	assert.Equal(t, `{"id":"ekv_abc","d":"c29tZWRhdGE="}`, v)
}

func TestEncryptedField_Scan_JSON(t *testing.T) {
	var ef EncryptedField
	err := ef.Scan(`{"id":"ekv_abc","d":"c29tZWRhdGE="}`)
	require.NoError(t, err)
	assert.Equal(t, apid.ID("ekv_abc"), ef.ID)
	assert.Equal(t, "c29tZWRhdGE=", ef.Data)
}

func TestEncryptedField_Scan_JSONBytes(t *testing.T) {
	var ef EncryptedField
	err := ef.Scan([]byte(`{"id":"ekv_xyz","d":"dGVzdA=="}`))
	require.NoError(t, err)
	assert.Equal(t, apid.ID("ekv_xyz"), ef.ID)
	assert.Equal(t, "dGVzdA==", ef.Data)
}

func TestEncryptedField_Scan_Nil(t *testing.T) {
	ef := EncryptedField{ID: "ekv_abc", Data: "old"}
	err := ef.Scan(nil)
	require.NoError(t, err)
	assert.True(t, ef.IsZero())
}

func TestEncryptedField_Scan_EmptyString(t *testing.T) {
	var ef EncryptedField
	err := ef.Scan("")
	require.NoError(t, err)
	assert.True(t, ef.IsZero())
}

func TestEncryptedField_Scan_EmptyBytes(t *testing.T) {
	var ef EncryptedField
	err := ef.Scan([]byte{})
	require.NoError(t, err)
	assert.True(t, ef.IsZero())
}

func TestEncryptedField_Scan_InvalidType(t *testing.T) {
	var ef EncryptedField
	err := ef.Scan(123)
	assert.Error(t, err)
}

func TestEncryptedField_ValueRoundTrip(t *testing.T) {
	original := EncryptedField{ID: "ekv_roundtrip", Data: "YWJjZGVm"}
	v, err := original.Value()
	require.NoError(t, err)

	var restored EncryptedField
	err = restored.Scan(v)
	require.NoError(t, err)
	assert.Equal(t, original, restored)
}

func TestEncryptedField_Equal(t *testing.T) {
	var ef1 *EncryptedField
	var ef2 *EncryptedField
	assert.True(t, ef1.Equal(ef2))

	ef1 = &EncryptedField{ID: "ekv_abc", Data: "data"}
	assert.False(t, ef2.Equal(ef1))
	assert.False(t, ef1.Equal(nil))

	ef2 = &EncryptedField{ID: "ekv_abc", Data: "data"}
	assert.True(t, ef1.Equal(ef2))

	ef2 = &EncryptedField{ID: "ekv_abc", Data: "other"}
	assert.False(t, ef1.Equal(ef2))
}

func TestParseInlineString(t *testing.T) {
	ef, err := ParseInlineString("ekv_abc123:c29tZWRhdGE=")
	require.NoError(t, err)
	assert.Equal(t, apid.ID("ekv_abc123"), ef.ID)
	assert.Equal(t, "c29tZWRhdGE=", ef.Data)
}

func TestParseInlineString_MissingPrefix(t *testing.T) {
	_, err := ParseInlineString("abc:data")
	assert.Error(t, err)
}

func TestParseInlineString_MissingColon(t *testing.T) {
	_, err := ParseInlineString("ekv_abcnodata")
	assert.Error(t, err)
}

func TestToInlineString_Roundtrips(t *testing.T) {
	ef := EncryptedField{ID: "ekv_abc", Data: "data"}
	rt, err := ParseInlineString(ef.ToInlineString())
	assert.NoError(t, err)
	assert.Equal(t, ef, rt)
}
