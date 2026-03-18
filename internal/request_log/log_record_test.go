package request_log

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/stretchr/testify/require"
)

func TestLogRecord(t *testing.T) {
	val := LogRecord{
		Type:                httpf.RequestTypeOAuth,
		Namespace:           "root.child",
		RequestId:           apid.New(apid.PrefixRequestLog),
		CorrelationId:       "some-correlation-id",
		Timestamp:           time.Date(1970, time.January, 1, 0, 20, 34, 567000000, time.UTC), // This only has millisecond precision
		MillisecondDuration: MillisecondDuration(2 * time.Second),
		ConnectionId:        apid.New(apid.PrefixConnection),
		ConnectorId:         apid.New(apid.PrefixConnectorVersion),
		ConnectorVersion:    7,
		Method:              "GET",
		Host:                "example.com",
		Scheme:              "http",
		Path:                "/example",
		RequestHttpVersion:  "HTTP/1.1",
		RequestSizeBytes:    123,
		RequestMimeType:     "text/plain",
		ResponseStatusCode:  200,
		ResponseError:       "some error",
		ResponseHttpVersion: "HTTP/1.1",
		ResponseSizeBytes:   321,
		ResponseMimeType:    "text/html",
		InternalTimeout:     true,
		RequestCancelled:    true,
		FullRequestRecorded: true,
	}

	t.Run("it roundtrips as json", func(t *testing.T) {
		data, err := json.Marshal(val)
		require.NoError(t, err)

		result := LogRecord{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)
		require.Equal(t, val, result)
	})

	t.Run("it roundtrips as json with labels", func(t *testing.T) {
		withLabels := val
		withLabels.Labels = database.Labels{"env": "prod", "team": "api"}

		data, err := json.Marshal(withLabels)
		require.NoError(t, err)

		// Verify labels appear in JSON
		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)
		labels := raw["labels"].(map[string]interface{})
		require.Equal(t, "prod", labels["env"])
		require.Equal(t, "api", labels["team"])

		// Roundtrip
		result := LogRecord{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)
		require.Equal(t, withLabels, result)
	})

	t.Run("labels omitted from json when nil", func(t *testing.T) {
		noLabels := val
		noLabels.Labels = nil

		data, err := json.Marshal(noLabels)
		require.NoError(t, err)

		var raw map[string]interface{}
		err = json.Unmarshal(data, &raw)
		require.NoError(t, err)
		_, hasLabels := raw["labels"]
		require.False(t, hasLabels, "labels should be omitted from JSON when nil")
	})
}
