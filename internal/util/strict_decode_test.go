package util

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type strictDecodePayload struct {
	CamelCase string `json:"camelCase" yaml:"camelCase"`
}

type strictInitialismPayload struct {
	ConnectorID string `json:"connectorId" yaml:"connectorId"`
	ReturnToURL string `json:"returnToUrl" yaml:"returnToUrl"`
	BodyJSON    string `json:"bodyJson" yaml:"bodyJson"`
	TLS         bool   `json:"tls" yaml:"tls"`
	OAuth       bool   `json:"oauth" yaml:"oauth"`
}

func TestDecodeJSONStrict(t *testing.T) {
	var payload strictDecodePayload
	require.NoError(t, DecodeJSONStrict([]byte(`{"camelCase":"ok"}`), &payload))
	require.Error(t, DecodeJSONStrict([]byte(`{"snake_case":"no"}`), &payload))
	require.Error(t, DecodeJSONStrict([]byte(`{"camelCase":"ok"}{}`), &payload))
}

func TestDecodeYAMLStrict(t *testing.T) {
	var payload strictDecodePayload
	require.NoError(t, DecodeYAMLStrict([]byte("camelCase: ok\n"), &payload))
	require.Error(t, DecodeYAMLStrict([]byte("snake_case: no\n"), &payload))
	require.Error(t, DecodeYAMLStrict([]byte("camelCase: ok\n---\ncamelCase: again\n"), &payload))
}

func TestContractInitialismCasing(t *testing.T) {
	payload := strictInitialismPayload{
		ConnectorID: "cxr_1",
		ReturnToURL: "https://example.com/return",
		BodyJSON:    "{}",
		TLS:         true,
		OAuth:       true,
	}

	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.JSONEq(t, `{"connectorId":"cxr_1","returnToUrl":"https://example.com/return","bodyJson":"{}","tls":true,"oauth":true}`, string(data))
}
