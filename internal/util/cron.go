package util

import (
	"fmt"
	"time"
)

// DurationToCronSpec converts a time.Duration to a cron specification
// that represents recurring events at that interval
func DurationToCronSpec(d time.Duration) (string, error) {
	seconds := int(d.Seconds())
	minutes := int(d.Minutes())
	hours := int(d.Hours())

	// Handle different duration ranges
	switch {
	// For durations less than 1 minute
	case seconds < 60 && seconds > 0:
		return fmt.Sprintf("*/%d * * * * *", seconds), nil

	// For durations less than 1 hour (in minutes)
	case seconds < 3600 && minutes > 0:
		// If perfectly divisible by a minute
		if seconds%60 == 0 {
			return fmt.Sprintf("0 */%d * * * *", minutes), nil
		}

	// For durations less than 1 day (in hours)
	case seconds < 86400 && hours > 0:
		// If perfectly divisible by an hour
		if seconds%3600 == 0 {
			return fmt.Sprintf("0 0 */%d * * *", hours), nil
		}

	// For durations of exactly 1 day
	case seconds == 86400:
		return "0 0 0 * * *", nil
	}

	return "", fmt.Errorf("duration %v cannot be accurately expressed as cron spec", d)
}
