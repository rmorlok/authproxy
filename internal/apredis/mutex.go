package apredis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bsm/redislock"
	"github.com/rmorlok/authproxy/internal/apctx"
)

type Mutex interface {
	Lock(context.Context) error
	Extend(context.Context, time.Duration) error
	Unlock(context.Context) error
}

var (
	// ErrMutexNotLocked is returned when an operation requires an acquired mutex.
	ErrMutexNotLocked = errors.New("mutex not locked")
	// ErrMutexLeaseLost indicates that a mutex could not be refreshed and its
	// exclusive ownership can no longer be guaranteed.
	ErrMutexLeaseLost = errors.New("mutex lease lost")
)

const mutexCleanupTimeout = 5 * time.Second

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
			// context.WithTimeout automatically preserves an earlier parent deadline.
			return context.WithTimeout(ctx, d)
		}
	}
}

// NewMutex creates a new mutex. Unless options are specified this mutex will not retry to obtain the lock. The
// default lock time is 1 minute.
func NewMutex(r Client, key string, options ...MutexOption) Mutex {
	m := &mutex{
		key:             key,
		lockClient:      redislock.New(r),
		initialLockTime: 1 * time.Minute,
	}

	for _, option := range options {
		option(m)
	}

	return m
}

func MutexIsErrNotObtained(err error) bool {
	return errors.Is(err, redislock.ErrNotObtained)
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
		return fmt.Errorf("mutex '%s' already locked", m.key)
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
		return fmt.Errorf("mutex '%s': %w", m.key, ErrMutexNotLocked)
	}

	err := m.lock.Refresh(ctx, d, m.opts())
	if errors.Is(err, redislock.ErrNotObtained) {
		// Redis confirmed that this token no longer owns the lock. Preserve the
		// handle for transient transport errors so a later release can still be
		// attempted safely with the same token.
		m.lock = nil
	}

	return err
}

func (m *mutex) Unlock(ctx context.Context) error {
	if m.lock == nil {
		return fmt.Errorf("mutex '%s': %w", m.key, ErrMutexNotLocked)
	}

	err := m.lock.Release(ctx)
	if err == nil || errors.Is(err, redislock.ErrLockNotHeld) {
		m.lock = nil
	}
	return err
}

// RunWithMutex executes operation while continuously refreshing an acquired
// Redis mutex. If the lease can no longer be refreshed, the operation context
// is cancelled and ErrMutexLeaseLost is returned. Renewal is decoupled from
// parent cancellation and remains active until the operation returns, giving
// the operation time to stop without silently outliving the lease.
func RunWithMutex(
	ctx context.Context,
	m Mutex,
	leaseDuration time.Duration,
	operation func(context.Context) error,
) (resultErr error) {
	if m == nil {
		return errors.New("mutex is required")
	}
	if leaseDuration <= 0 {
		return fmt.Errorf("mutex lease duration must be greater than zero: %s", leaseDuration)
	}
	if operation == nil {
		return errors.New("mutex operation is required")
	}

	if err := m.Lock(ctx); err != nil {
		return fmt.Errorf("failed to acquire mutex: %w", err)
	}

	operationCtx, cancelOperation := context.WithCancelCause(ctx)
	renewCtx, stopRenewal := context.WithCancel(context.WithoutCancel(ctx))
	renewalDone := make(chan error, 1)
	renewalReady := make(chan struct{})
	go func() {
		renewalDone <- renewMutex(renewCtx, m, leaseDuration, cancelOperation, renewalReady)
	}()
	<-renewalReady

	defer func() {
		panicValue := recover()

		stopRenewal()
		renewalErr := <-renewalDone
		cancelOperation(context.Canceled)

		cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), mutexCleanupTimeout)
		unlockErr := m.Unlock(cleanupCtx)
		cancelCleanup()

		// A confirmed lease loss clears the mutex handle. The resulting
		// ErrMutexNotLocked does not add information beyond ErrMutexLeaseLost.
		if errors.Is(renewalErr, ErrMutexLeaseLost) && errors.Is(unlockErr, ErrMutexNotLocked) {
			unlockErr = nil
		}

		if renewalErr != nil {
			resultErr = errors.Join(resultErr, renewalErr)
		}
		if unlockErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("failed to release mutex: %w", unlockErr))
		}

		if panicValue != nil {
			panic(panicValue)
		}
	}()

	resultErr = operation(operationCtx)
	return resultErr
}

func renewMutex(
	ctx context.Context,
	m Mutex,
	leaseDuration time.Duration,
	cancelOperation context.CancelCauseFunc,
	ready chan<- struct{},
) error {
	refreshInterval := leaseDuration / 3
	if refreshInterval <= 0 {
		refreshInterval = leaseDuration
	}

	timer := apctx.GetClock(ctx).NewTimer(refreshInterval)
	defer timer.Stop()
	close(ready)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C():
			refreshCtx, cancelRefresh := context.WithTimeout(ctx, refreshInterval)
			err := m.Extend(refreshCtx, leaseDuration)
			cancelRefresh()
			if err != nil {
				leaseErr := fmt.Errorf("%w: failed to refresh mutex: %w", ErrMutexLeaseLost, err)
				cancelOperation(leaseErr)
				return leaseErr
			}
			timer.Reset(refreshInterval)
		}
	}
}
