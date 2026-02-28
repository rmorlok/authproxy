package util

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ParseTimestampRange parses the range string and returns start, end values.
func ParseTimestampRange(r string) (time.Time, time.Time, error) {
	if r == "" {
		return time.Time{}, time.Time{}, errors.New("no value specified for timestamp range")
	}

	if strings.Index(r, "-") == -1 {
		return time.Time{}, time.Time{}, errors.New("no range separator in timestamp range")
	}

	if strings.Count(r, "-") != 5 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid timestamp range format: '%s'; must be YYYY-MM-DDTHH:MM:SSZ-YYYY-MM-DDTHH:MM:SSZ", r)
	}

	// Find the third dash which separates the two timestamps
	firstDash := strings.Index(r, "-")
	secondDash := strings.Index(r[firstDash+1:], "-") + firstDash + 1
	thirdDashIndex := strings.Index(r[secondDash+1:], "-") + secondDash + 1

	startStr := r[:thirdDashIndex]
	endStr := r[thirdDashIndex+1:]

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid timestamp range format; invalid start timestamp format: '%s'; must be RFC3339", startStr)
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid timestamp range format; invalid end timestamp format: '%s'; must be RFC3339", endStr)
	}

	return start, end, nil
}
