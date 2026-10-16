package oauth2

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apredis"
)

// tokenMutex returns the per-connection Redis mutex that serializes every
// refresh of the connection's OAuth2 token across processes.
//
// The lease is the connector's refresh timeout, but refreshes run under
// apredis.RunWithMutex, which renews the lease for as long as the refresh is
// in flight. The lease length therefore only bounds how quickly a crashed
// holder's lock is reclaimed, not how long a refresh may take.
//
// Waiters retry for refreshLockWaitBudget: long enough to outlast a holder
// that uses its whole retry budget. Giving up earlier would surface a
// transient error to the caller even though the holder is about to persist a
// fresh token; waiting is cheap because the waiter re-reads the token once it
// acquires the lock and usually finds nothing left to do.
func (o *oAuth2Connection) tokenMutex() apredis.Mutex {
	return apredis.NewMutex(
		o.r,
		"oauth2-token-"+o.connection.GetId().String(),
		apredis.MutexOptionRetryFor(refreshLockWaitBudget(o.auth.Token.GetRefreshTimeout())),
		apredis.MutexOptionLockFor(o.auth.Token.GetRefreshTimeout()),
		apredis.MutexOptionRetryExponentialBackoff(50*time.Millisecond, 1*time.Second),
		apredis.MutexOptionDetailedLockMetadata(),
	)
}

// refreshLockWaitBudget returns how long a refresher waits to acquire the
// token mutex: the worst case for a holder that times out on every
// token-endpoint attempt (tokenRefreshMaxAttempts × refreshTimeout) plus the
// linear backoff between those attempts, plus one more refreshTimeout of slack
// for the holder's decrypt/persist work.
func refreshLockWaitBudget(refreshTimeout time.Duration) time.Duration {
	budget := time.Duration(tokenRefreshMaxAttempts+1) * refreshTimeout
	for i := 1; i < tokenRefreshMaxAttempts; i++ {
		budget += time.Duration(i) * tokenRefreshBackoffStep
	}
	return budget
}
