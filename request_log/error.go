package request_log

import "github.com/pkg/errors"

// ErrNotFound is returned when a log record is not found that was requested. This isn't necessarily a bad request
// as the record may have expired due to TTL.
var ErrNotFound = errors.New("record not found")
