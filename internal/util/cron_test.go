package util

import (
	"testing"
	"time"
)

func TestDurationToCronSpec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		duration  time.Duration
		want      string
		expectErr bool
	}{
		{
			name:     "less than a minute",
			duration: 10 * time.Second,
			want:     "*/10 * * * * *",
		},
		{
			name:     "exactly one minute",
			duration: 1 * time.Minute,
			want:     "0 */1 * * * *",
		},
		{
			name:     "multiple of minutes (5 minutes)",
			duration: 5 * time.Minute,
			want:     "0 */5 * * * *",
		},
		{
			name:     "exactly one hour",
			duration: 1 * time.Hour,
			want:     "0 0 */1 * * *",
		},
		{
			name:     "multiple of hours (3 hours)",
			duration: 3 * time.Hour,
			want:     "0 0 */3 * * *",
		},
		{
			name:     "exactly one day (24 hours)",
			duration: 24 * time.Hour,
			want:     "0 0 0 * * *",
		},
		{
			name:      "non divisible seconds (70 seconds)",
			duration:  70 * time.Second,
			expectErr: true,
		},
		{
			name:      "non divisible minutes (1 hour 5 minutes)",
			duration:  65 * time.Minute,
			expectErr: true,
		},
		{
			name:      "non divisible hours (1 day 3 hours)",
			duration:  27 * time.Hour,
			expectErr: true,
		},
		{
			name:      "zero duration",
			duration:  0 * time.Second,
			expectErr: true,
		},
		{
			name:      "negative duration",
			duration:  -5 * time.Minute,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DurationToCronSpec(tt.duration)
			if (err != nil) != tt.expectErr {
				t.Fatalf("DurationToCronSpec() error = %v, expectErr %v", err, tt.expectErr)
			}
			if got != tt.want {
				t.Errorf("DurationToCronSpec() got = %v, want %v", got, tt.want)
			}
		})
	}
}
