package request_log

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestSetRedisRecordFields(t *testing.T) {
	tests := []struct {
		name     string
		entry    *Entry
		expected EntryRecord
	}{
		{
			name: "Valid Entry and Response",
			entry: &Entry{
				ID:                  uuid.New(),
				CorrelationID:       "test-correlation-id",
				Timestamp:           time.Now(),
				MillisecondDuration: 500,
				Request: EntryRequest{
					URL:         "https://example.com",
					HttpVersion: "HTTP/1.1",
					Method:      "GET",
					Headers: map[string][]string{
						"Content-Type": {"application/json"},
					},
					ContentLength: 1234,
					Body:          []byte("sample request body"),
				},
				Response: EntryResponse{
					HttpVersion:   "HTTP/1.1",
					StatusCode:    200,
					Headers:       map[string][]string{"Content-Type": {"application/json"}},
					ContentLength: 5678,
					Body:          []byte("sample response body"),
				},
			},
			expected: EntryRecord{
				RequestId:           uuid.New(), // Will be overridden during test
				CorrelationId:       "test-correlation-id",
				Timestamp:           time.Now(), // Will be overridden during test
				MillisecondDuration: 500,
				Method:              "GET",
				Scheme:              "https",
				Host:                "example.com",
				Path:                "",
				RequestHttpVersion:  "HTTP/1.1",
				RequestSizeBytes:    1234,
				RequestMimeType:     "application/json",
				ResponseStatusCode:  200,
				ResponseMimeType:    "application/json",
				ResponseSizeBytes:   5678,
				ResponseHttpVersion: "HTTP/1.1",
			},
		},
		{
			name:  "Nil Entry",
			entry: nil,
			expected: EntryRecord{
				// Expect no fields set
			},
		},
		{
			name: "Empty Request and Response",
			entry: &Entry{
				ID:                  uuid.New(),
				CorrelationID:       "test-empty",
				Timestamp:           time.Now(),
				MillisecondDuration: 1000,
			},
			expected: EntryRecord{
				RequestId:           uuid.New(), // Will be overridden during test
				CorrelationId:       "test-empty",
				Timestamp:           time.Now(), // Will be overridden during test
				MillisecondDuration: 1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryRecord := EntryRecord{}
			if tt.entry != nil {
				tt.expected.RequestId = tt.entry.ID
				tt.expected.Timestamp = tt.entry.Timestamp
			}

			tt.entry.setRedisRecordFields(&entryRecord)
			assert.Equal(t, tt.expected.RequestId, entryRecord.RequestId)
			assert.Equal(t, tt.expected.CorrelationId, entryRecord.CorrelationId)
			assert.Equal(t, tt.expected.Timestamp, entryRecord.Timestamp)
			assert.Equal(t, tt.expected.MillisecondDuration, entryRecord.MillisecondDuration)
			assert.Equal(t, tt.expected.Method, entryRecord.Method)
			assert.Equal(t, tt.expected.Scheme, entryRecord.Scheme)
			assert.Equal(t, tt.expected.Host, entryRecord.Host)
			assert.Equal(t, tt.expected.Path, entryRecord.Path)
			assert.Equal(t, tt.expected.RequestHttpVersion, entryRecord.RequestHttpVersion)
			assert.Equal(t, tt.expected.ResponseHttpVersion, entryRecord.ResponseHttpVersion)
			assert.Equal(t, tt.expected.ResponseSizeBytes, entryRecord.ResponseSizeBytes)
			assert.Equal(t, tt.expected.ResponseStatusCode, entryRecord.ResponseStatusCode)
		})
	}
}
