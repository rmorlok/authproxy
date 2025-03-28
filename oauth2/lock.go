package oauth2

import (
	"github.com/rmorlok/authproxy/redis"
	"time"
)

func (o *OAuth2) tokenMutex() redis.Mutex {
	return o.redis.NewMutex(
		"oauth2-token-"+o.connection.ID.String(),
		redis.MutexOptionRetryFor(o.auth.Token.GetRefreshTimeout()),
		redis.MutexOptionLockFor(o.auth.Token.GetRefreshTimeout()),
		redis.MutexOptionRetryExponentialBackoff(50*time.Millisecond, 1*time.Second),
		redis.MutexOptionDetailedLockMetadata(),
	)
}
