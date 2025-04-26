package redis

import (
	"context"
	"github.com/bsm/redislock"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"time"
)

type Mutex interface {
	Lock(context.Context) error
	Extend(context.Context, time.Duration) error
	Unlock(context.Context) error
}

type MutexOption func(m *mutex)

// MutexOptionLockFor sets the initial lock duration for the mutex. If unspecified, the default initial lock duration
// is one minute. This duration can be extended by calling Extend(...) on the mutex once it's acquired.
func MutexOptionLockFor(d time.Duration) MutexOption {
	return func(m *mutex) {
		m.initialLockTime = d
	}
}

// MutexOptionLockToken sets the token value used with the key when obtaining the lock in Redis. Setting this value
// explicitly allows you to control how the lock is inspectable in the redis data. By default, a random value is used.
// If you set this value, you need to be careful that you understand how you expect the lock to behave across processes
// and within the same process. For debugging, setting metadata as an appended value on the lock value may be a better
// option.
func MutexOptionLockToken(token string) MutexOption {
	return func(m *mutex) {
		m.optsAppliers = append(m.optsAppliers, func(opts *redislock.Options) {
			opts.Token = token
		})
	}
}

// MutexOptionLockMetadata appends additional data to the value used to obtain the lock in redis for debugging
// purposes.
func MutexOptionLockMetadata(metadata string) MutexOption {
	return func(m *mutex) {
		m.optsAppliers = append(m.optsAppliers, func(opts *redislock.Options) {
			opts.Metadata = metadata
		})
	}
}

// MutexOptionDetailedLockMetadata applies json metadata to the lock about the host, process, etc that acquired
// the lock
func MutexOptionDetailedLockMetadata() MutexOption {
	return MutexOptionLockMetadata(generateDetailedLockValue())
}

func MutexOptionRetryLinearBackoff(backoff time.Duration) MutexOption {
	return func(m *mutex) {
		m.optsAppliers = append(m.optsAppliers, func(opts *redislock.Options) {
			opts.RetryStrategy = redislock.LinearBackoff(backoff)
		})
	}
}

func MutexOptionRetryExponentialBackoff(min, max time.Duration) MutexOption {
	return func(m *mutex) {
		m.optsAppliers = append(m.optsAppliers, func(opts *redislock.Options) {
			opts.RetryStrategy = redislock.ExponentialBackoff(min, max)
		})
	}
}

func MutexOptionRetryForLinearBackoff(tries int, backoff time.Duration) MutexOption {
	return func(m *mutex) {
		m.optsAppliers = append(m.optsAppliers, func(opts *redislock.Options) {
			opts.RetryStrategy = redislock.LimitRetry(redislock.LinearBackoff(backoff), tries)
		})
	}
}

func MutexOptionRetryForExponentialBackoff(tries int, min, max time.Duration) MutexOption {
	return func(m *mutex) {
		m.optsAppliers = append(m.optsAppliers, func(opts *redislock.Options) {
			opts.RetryStrategy = redislock.LimitRetry(redislock.ExponentialBackoff(min, max), tries)
		})
	}
}

func MutexOptionNoRetry() MutexOption {
	return func(m *mutex) {
		m.optsAppliers = append(m.optsAppliers, func(opts *redislock.Options) {
			opts.RetryStrategy = redislock.NoRetry()
		})
	}
}

// MutexOptionRetryFor sets how long the mutex will attempt to retry for a lock. This must be combined with a retry
// strategy or the default MutexOptionNoRetry() will prevent retries.
func MutexOptionRetryFor(d time.Duration) MutexOption {
	return func(m *mutex) {
		m.lockContextCancellation = func(ctx context.Context) (context.Context, context.CancelFunc) {
			// Check if the context currently has a more aggressive deadline, and if so, respect that
			if currentDeadline, ok := ctx.Deadline(); !ok {
				desiredDeadline := apctx.GetClock(ctx).Now().Add(d)
				if desiredDeadline.After(currentDeadline) {
					return ctx, func() {}
				}
			}

			return context.WithTimeout(ctx, d)
		}
	}
}

// NewMutex creates a new mutex. Unless options are specified this mutex will not retry to obtain the lock. The
// default lock time is 1 minute.
func (w *wrapper) NewMutex(key string, options ...MutexOption) Mutex {
	m := &mutex{
		key:             key,
		lockClient:      w.lockClient,
		initialLockTime: 1 * time.Minute,
	}

	for _, option := range options {
		option(m)
	}

	return m
}

func MutexIsErrNotObtained(err error) bool {
	return err == redislock.ErrNotObtained
}

type mutex struct {
	key                     string
	lockContextCancellation func(context.Context) (context.Context, context.CancelFunc)
	lock                    *redislock.Lock
	lockClient              *redislock.Client
	initialLockTime         time.Duration
	optsAppliers            []func(*redislock.Options)
}

func (m *mutex) opts() *redislock.Options {
	if len(m.optsAppliers) == 0 {
		return nil
	}

	opts := &redislock.Options{}
	for _, applier := range m.optsAppliers {
		applier(opts)
	}

	return opts
}

func (m *mutex) Lock(ctx context.Context) error {
	if m.lock != nil {
		return errors.Errorf("mutex '%s' already locked", m.key)
	}

	if m.lockContextCancellation != nil {
		var cancel context.CancelFunc
		ctx, cancel = m.lockContextCancellation(ctx)
		defer cancel()
	}

	var err error
	m.lock, err = m.lockClient.Obtain(ctx, m.key, m.initialLockTime, m.opts())
	return err
}

func (m *mutex) Extend(ctx context.Context, d time.Duration) error {
	if m.lock == nil {
		return errors.Errorf("mutex '%s' not locked", m.key)
	}

	return m.lock.Refresh(ctx, d, m.opts())
}

func (m *mutex) Unlock(ctx context.Context) error {
	if m.lock == nil {
		return errors.Errorf("mutex '%s' not locked", m.key)
	}

	return m.lock.Release(ctx)
}
