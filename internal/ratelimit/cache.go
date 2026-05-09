package ratelimit

import (
	"sync"
	"time"

	"github.com/rmorlok/authproxy/internal/database"
)

// Cache is the read-side of the in-memory rate-limit rule cache. The
// proxy-path enforcement layer (#223) consumes this interface to look up
// matching rules at request time without round-tripping to the database.
//
// Reads are safe for concurrent use; values returned from All() are a
// snapshot — callers must not mutate them, and a subsequent Replace() will
// not affect the slice they hold.
type Cache interface {
	// All returns every rule in the current snapshot. The returned slice is
	// owned by the caller and won't be mutated by concurrent Replace() calls.
	All() []*database.RateLimit

	// SnapshotTime is when the current snapshot was last installed via
	// Replace(). Zero if no snapshot has been installed yet.
	SnapshotTime() time.Time

	// SnapshotVersion increments each time a snapshot is installed. Useful
	// for callers that want to debounce repeated cache reads or detect
	// staleness.
	SnapshotVersion() uint64
}

// MutableCache extends Cache with the write entry-point used by the Refresher.
// Kept separate from Cache so the enforcement layer's dependency surface stays
// read-only.
type MutableCache interface {
	Cache

	// Replace atomically swaps the snapshot. The supplied slice is taken as-is
	// — callers must not mutate it after the call.
	Replace(rules []*database.RateLimit, at time.Time)
}

type cache struct {
	mu      sync.RWMutex
	rules   []*database.RateLimit
	at      time.Time
	version uint64
}

// NewCache returns a fresh, empty in-memory cache.
func NewCache() *cache {
	return &cache{}
}

func (c *cache) All() []*database.RateLimit {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.rules) == 0 {
		return nil
	}
	// Copy to insulate callers from a concurrent Replace().
	out := make([]*database.RateLimit, len(c.rules))
	copy(out, c.rules)
	return out
}

func (c *cache) SnapshotTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.at
}

func (c *cache) SnapshotVersion() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

func (c *cache) Replace(rules []*database.RateLimit, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rules = rules
	c.at = at
	c.version++
}

var (
	_ Cache        = (*cache)(nil)
	_ MutableCache = (*cache)(nil)
)
