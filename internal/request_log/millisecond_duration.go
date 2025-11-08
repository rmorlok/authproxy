package request_log

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// MillisecondDuration is a wrapper around time.Duration for JSON marshaling as milliseconds
type MillisecondDuration time.Duration

// MarshalJSON implements the json.Marshaler interface
func (d MillisecondDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).Milliseconds())
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (d *MillisecondDuration) UnmarshalJSON(b []byte) error {
	var ms int64
	if err := json.Unmarshal(b, &ms); err != nil {
		return err
	}
	*d = MillisecondDuration(time.Duration(ms) * time.Millisecond)
	return nil
}

func (d MillisecondDuration) Duration() time.Duration {
	return time.Duration(d)
}

func (d MillisecondDuration) String() string {
	return strconv.FormatInt(d.Duration().Milliseconds(), 10)
}

// ParseMillisecondDuration parses a string representing a duration in milliseconds and returns the duration in
// time.Duration format.
func parseMillisecondDuration(s string) (MillisecondDuration, error) {
	if dur, err := time.ParseDuration(s + "ms"); err != nil {
		return MillisecondDuration(0), errors.Wrap(err, "failed to parse duration")
	} else {
		return MillisecondDuration(dur), nil
	}
}
