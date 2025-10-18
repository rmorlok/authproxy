package request_log

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/util"
	"github.com/stretchr/testify/assert"
)

func TestWithParsedTimestampRange(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedError    error
		expectedRange    []time.Time
		expectExactMatch bool
	}{
		{
			name:          "Valid range of times",
			input:         "2025-10-18T00:00:00Z-2025-10-19T23:59:59Z",
			expectedError: nil,
			expectedRange: []time.Time{
				util.Must(time.Parse(time.RFC3339, "2025-10-18T00:00:00Z")),
				util.Must(time.Parse(time.RFC3339, "2025-10-19T23:59:59Z")),
			},
			expectExactMatch: false,
		},
		{
			name:             "Empty input",
			input:            "",
			expectedError:    errors.New("no value specified for timestamp range"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with multiple dashes",
			input:            "2025-10-18T00:00:00Z-2025-10-18T00:00:00Z-2025-10-18T00:00:00Z",
			expectedError:    errors.New("invalid timestamp range format"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with multiple consecutive dashes",
			input:            "2025-10-18T00:00:00Z-2025-10--18T00:00:00Z-2025-10-18T00:00:00Z",
			expectedError:    errors.New("invalid timestamp range format"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with non-timestamp value",
			input:            "200-300",
			expectedError:    errors.New("invalid timestamp range format"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with alphabetical characters",
			input:            "abc",
			expectedError:    errors.New("invalid timestamp range format"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Empty range end value",
			input:            "2025-10-18T00:00:00Z-",
			expectedError:    errors.New("invalid timestamp range format"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Empty range start value",
			input:            "-2025-10-18T00:00:00Z",
			expectedError:    errors.New("invalid timestamp range format"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &listRequestsFilters{}

			builder, err := filter.WithParsedTimestampRange(tt.input)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.True(t, api_common.HttpStatusErrorContains(err, tt.expectedError.Error()))
				assert.True(t, api_common.HttpStatusErrorIsStatusCode(err, http.StatusBadRequest))
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, builder)

				if tt.expectExactMatch {
					assert.Len(t, filter.TimestampRange, 2)
					assert.Equal(t, tt.expectedRange[0], filter.TimestampRange[0])
					assert.Equal(t, tt.expectedRange[1], filter.TimestampRange[1])
				} else {
					assert.Equal(t, tt.expectedRange, filter.TimestampRange)
				}
			}
		})
	}
}

func TestWithParsedStatusCodeRange(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedError    error
		expectedRange    []int
		expectExactMatch bool
	}{
		{
			name:             "Valid exact status code",
			input:            "200",
			expectedError:    nil,
			expectedRange:    []int{200, 200},
			expectExactMatch: true,
		},
		{
			name:             "Valid range of status codes",
			input:            "200-299",
			expectedError:    nil,
			expectedRange:    []int{200, 299},
			expectExactMatch: false,
		},
		{
			name:             "Empty input",
			input:            "",
			expectedError:    errors.New("no value specified for status code ranges"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with multiple dashes",
			input:            "200-300-400",
			expectedError:    errors.New("more than one dash in status code range"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with multiple consecutive dashes",
			input:            "200---400",
			expectedError:    errors.New("more than one dash in status code range"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with non-numeric value",
			input:            "20a-300",
			expectedError:    errors.New("cannot parse value"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Invalid format with alphabetical characters",
			input:            "abc",
			expectedError:    errors.New("cannot parse value"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Empty range end value",
			input:            "200-",
			expectedError:    errors.New("cannot parse value"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
		{
			name:             "Empty range start value",
			input:            "-300",
			expectedError:    errors.New("cannot parse value"),
			expectedRange:    nil,
			expectExactMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := &listRequestsFilters{}

			builder, err := filter.WithParsedStatusCodeRange(tt.input)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.True(t, api_common.HttpStatusErrorContains(err, tt.expectedError.Error()))
				assert.True(t, api_common.HttpStatusErrorIsStatusCode(err, http.StatusBadRequest))
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, builder)

				if tt.expectExactMatch {
					assert.Len(t, filter.StatusCodeRangeInclusive, 2)
					assert.Equal(t, tt.expectedRange[0], filter.StatusCodeRangeInclusive[0])
					assert.Equal(t, tt.expectedRange[1], filter.StatusCodeRangeInclusive[1])
				} else {
					assert.Equal(t, tt.expectedRange, filter.StatusCodeRangeInclusive)
				}
			}
		})
	}
}
