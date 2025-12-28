package oauth2

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apredis"
)

func (o *oAuth2Connection) tokenMutex() apredis.Mutex {
	return apredis.NewMutex(
		o.r,
		"oauth2-token-"+o.connection.GetId().String(),
		apredis.MutexOptionRetryFor(o.auth.Token.GetRefreshTimeout()),
		apredis.MutexOptionLockFor(o.auth.Token.GetRefreshTimeout()),
		apredis.MutexOptionRetryExponentialBackoff(50*time.Millisecond, 1*time.Second),
		apredis.MutexOptionDetailedLockMetadata(),
	)
}
