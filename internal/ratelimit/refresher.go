package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	clock "k8s.io/utils/clock"
)

// DefaultRefreshInterval is how often the Refresher pulls a fresh snapshot
// from the database when no override is supplied.
const DefaultRefreshInterval = 5 * time.Minute

// minRefreshInterval guards against pathological config values that would
// hammer the database. Values below this floor are clamped at construction.
const minRefreshInterval = 5 * time.Second

// Refresher periodically pulls all non-deleted rate-limit definitions from
// the database into a MutableCache. One Refresher runs per proxy process; on
// process startup the api/admin-api server should call Sync() once to
// populate the cache before serving traffic, then Run() in a goroutine for
// the lifetime of the process.
//
// The DB query is performed without a Redis mutex: the cache is per-process
// and each proxy node refreshes independently of the others. Coordinating
// across processes via Redis (sentinel/mutex) would only help if the cache
// were shared (e.g., a Redis-backed snapshot read by all proxies) — that
// design is intentionally deferred until the proxy fleet is large enough to
// stress the database from synchronized refreshes.
type Refresher struct {
	db       database.DB
	cache    MutableCache
	interval time.Duration
	clock    clock.WithTicker
	logger   *slog.Logger

	// onSync is fired after every Sync() completes (success or failure). Used
	// only by tests; production code leaves this nil.
	onSync func(err error)
}

// RefresherOption configures a Refresher.
type RefresherOption func(*Refresher)

// WithInterval overrides DefaultRefreshInterval.
func WithInterval(d time.Duration) RefresherOption {
	return func(r *Refresher) {
		if d < minRefreshInterval {
			d = minRefreshInterval
		}
		r.interval = d
	}
}

// WithClock injects a clock for tests that need to drive the periodic loop
// deterministically. Production callers leave this default.
func WithClock(c clock.WithTicker) RefresherOption {
	return func(r *Refresher) {
		if c != nil {
			r.clock = c
		}
	}
}

// withOnSync is used by tests to observe Sync() completions.
func withOnSync(fn func(err error)) RefresherOption {
	return func(r *Refresher) {
		r.onSync = fn
	}
}

// NewRefresher constructs a Refresher. Both db and cache must be non-nil.
func NewRefresher(db database.DB, cache MutableCache, logger *slog.Logger, opts ...RefresherOption) *Refresher {
	if logger == nil {
		logger = slog.Default()
	}
	r := &Refresher{
		db:       db,
		cache:    cache,
		interval: DefaultRefreshInterval,
		clock:    clock.RealClock{},
		logger:   logger,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Sync performs a single fetch from the database and atomically swaps the
// cache. If the database query fails the cache is left as-is — the
// "last-known-good" property the enforcement layer depends on.
func (r *Refresher) Sync(ctx context.Context) error {
	start := r.clock.Now()

	rules, err := r.fetchAllRateLimits(ctx)
	if err != nil {
		r.logger.Error("rate-limit cache sync failed; keeping last-known-good snapshot",
			"error", err,
			"snapshot_version", r.cache.SnapshotVersion(),
			"snapshot_age", time.Since(r.cache.SnapshotTime()),
		)
		r.notifySync(err)
		return err
	}

	r.cache.Replace(rules, r.clock.Now())
	r.logger.Info("rate-limit cache refreshed",
		"rule_count", len(rules),
		"duration", r.clock.Now().Sub(start),
		"snapshot_version", r.cache.SnapshotVersion(),
	)
	r.notifySync(nil)
	return nil
}

// Run starts a periodic Sync loop that returns when ctx is cancelled. The
// initial Sync() is performed *before* the first tick so the cache is hot
// before the goroutine settles into its loop. Callers who need a guaranteed
// hot cache before serving traffic should call Sync() inline first; this
// goroutine's first iteration is a backstop.
func (r *Refresher) Run(ctx context.Context) error {
	if err := r.Sync(ctx); err != nil {
		// Continue regardless — the cache may already hold a previous
		// snapshot and we want the loop running for retry.
		r.logger.Warn("initial rate-limit cache sync failed", "error", err)
	}

	t := r.clock.NewTicker(r.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C():
			if err := r.Sync(ctx); err != nil {
				r.logger.Warn("rate-limit cache periodic sync failed", "error", err)
			}
		}
	}
}

func (r *Refresher) fetchAllRateLimits(ctx context.Context) ([]*database.RateLimit, error) {
	if r.db == nil {
		return nil, errors.New("ratelimit.Refresher: nil database")
	}

	var collected []*database.RateLimit
	err := r.db.ListRateLimitsBuilder().
		Limit(500).
		Enumerate(ctx, func(page pagination.PageResult[database.RateLimit]) (pagination.KeepGoing, error) {
			if page.Error != nil {
				return pagination.Stop, page.Error
			}
			for i := range page.Results {
				rl := page.Results[i]
				collected = append(collected, &rl)
			}
			return pagination.Continue, nil
		})
	if err != nil {
		return nil, fmt.Errorf("listing rate limits: %w", err)
	}
	return collected, nil
}

func (r *Refresher) notifySync(err error) {
	if r.onSync != nil {
		r.onSync(err)
	}
}

// Background entry-point used by api/admin-api server bootstrap. Returns a
// stop function that cancels the goroutine and waits for it to exit.
//
// Typical use:
//
//	stop := ratelimit.StartRefresher(ctx, db, cache, logger)
//	defer stop()
func StartRefresher(parentCtx context.Context, db database.DB, cache MutableCache, logger *slog.Logger, opts ...RefresherOption) (stop func()) {
	r := NewRefresher(db, cache, logger, opts...)
	ctx, cancel := context.WithCancel(parentCtx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = r.Run(ctx)
	}()
	return func() {
		cancel()
		wg.Wait()
	}
}

