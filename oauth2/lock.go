package oauth2

import (
	"github.com/rmorlok/authproxy/apredis"
	"time"
)

func (o *oAuth2Connection) tokenMutex() apredis.Mutex {
	return apredis.NewMutex(
		o.r,
		"oauth2-token-"+o.connection.ID.String(),
		apredis.MutexOptionRetryFor(o.auth.Token.GetRefreshTimeout()),
		apredis.MutexOptionLockFor(o.auth.Token.GetRefreshTimeout()),
		apredis.MutexOptionRetryExponentialBackoff(50*time.Millisecond, 1*time.Second),
		apredis.MutexOptionDetailedLockMetadata(),
	)
}
