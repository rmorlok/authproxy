package request_log

import "encoding/json"

// marshalRateLimitBucket renders the bucket map to a stable JSON form
// suitable for storage. An empty / nil map serializes to "{}" so the DB
// column always holds valid JSON (matches the migration default).
func marshalRateLimitBucket(b map[string]string) (string, error) {
	if len(b) == 0 {
		return "{}", nil
	}
	out, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// unmarshalRateLimitBucket parses a bucket JSON blob from the DB. nil /
// empty / "{}" all yield a nil map so callers don't need to special-case
// "missing" vs "explicitly empty" — semantically the same for log
// readers.
func unmarshalRateLimitBucket(data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// marshalRateLimitMatched renders the matched list to JSON. Empty / nil
// → "[]" so the DB column always holds valid JSON.
func marshalRateLimitMatched(m []RateLimitMatch) (string, error) {
	if len(m) == 0 {
		return "[]", nil
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// unmarshalRateLimitMatched parses a matched-list JSON blob. nil / empty
// / "[]" yield a nil slice.
func unmarshalRateLimitMatched(data []byte) ([]RateLimitMatch, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var out []RateLimitMatch
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
