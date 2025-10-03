package request_log

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEntryRecord(t *testing.T) {
	val := EntryRecord{
		Type:                RequestTypeOAuth,
		RequestId:           uuid.New(),
		CorrelationId:       "some-correlation-id",
		Timestamp:           time.Unix(0, time.Now().UnixMilli()*int64(time.Millisecond)), // There will be sub-millisecond loss of precision here
		MillisecondDuration: MillisecondDuration(2 * time.Second),
		ConnectionId:        uuid.New(),
		ConnectorType:       "some-type",
		ConnectorId:         uuid.New(),
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
	}

	t.Run("it roundtrips from redis fields", func(t *testing.T) {
		data := make(map[string]string)
		val.setRedisRecordFields(data)

		result, err := EntryRecordFromRedisFields(data)
		require.NoError(t, err)
		require.Equal(t, val, *result)

		result, err = EntryRecordFromRedisFields(nil)
		require.NoError(t, err)
		require.Nil(t, result)
	})

	t.Run("it roundtrips as json", func(t *testing.T) {
		data, err := json.Marshal(val)
		require.NoError(t, err)

		result := EntryRecord{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)
		require.Equal(t, val, result)
	})
}
