package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apredis"
)

const (
	redisKeyPrefix         = "ratelimit:"
	blockedKeyPrefix       = redisKeyPrefix + "blocked:"
	consecutive429KeyPrefix = redisKeyPrefix + "429count:"
	// consecutiveCountTTL is how long the consecutive 429 counter persists without activity.
	consecutiveCountTTL = 1 * time.Hour
)

// Store manages rate limiting state in Redis.
type Store struct {
	r apredis.Client
}

// NewStore creates a new rate limit store backed by Redis.
func NewStore(r apredis.Client) *Store {
	return &Store{r: r}
}

func blockedKey(connectionID apid.ID) string {
	return fmt.Sprintf("%s%s", blockedKeyPrefix, connectionID.String())
}

func consecutive429Key(connectionID apid.ID) string {
	return fmt.Sprintf("%s%s", consecutive429KeyPrefix, connectionID.String())
}

// IsRateLimited checks if a connection is currently rate-limited.
// Returns the remaining TTL and true if limited, or (0, false) if not.
func (s *Store) IsRateLimited(ctx context.Context, connectionID apid.ID) (time.Duration, bool, error) {
	ttl, err := s.r.TTL(ctx, blockedKey(connectionID)).Result()
	if err != nil {
		return 0, false, err
	}

	// TTL returns -2 if key doesn't exist, -1 if no TTL set
	if ttl <= 0 {
		return 0, false, nil
	}

	return ttl, true, nil
}

// SetRateLimited marks a connection as rate-limited for the given duration.
func (s *Store) SetRateLimited(ctx context.Context, connectionID apid.ID, duration time.Duration) error {
	return s.r.Set(ctx, blockedKey(connectionID), "1", duration).Err()
}

// GetConsecutive429Count returns the consecutive 429 count for exponential backoff.
func (s *Store) GetConsecutive429Count(ctx context.Context, connectionID apid.ID) (int, error) {
	val, err := s.r.Get(ctx, consecutive429Key(connectionID)).Result()
	if err != nil {
		if err.Error() == "redis: nil" {
			return 0, nil
		}
		return 0, err
	}

	count, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// IncrementConsecutive429Count increments the consecutive 429 counter and returns the new value.
func (s *Store) IncrementConsecutive429Count(ctx context.Context, connectionID apid.ID) (int, error) {
	key := consecutive429Key(connectionID)
	count, err := s.r.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// Reset the TTL on each increment
	s.r.Expire(ctx, key, consecutiveCountTTL)

	return int(count), nil
}

// ClearConsecutive429Count resets the counter (called on successful non-429 response).
func (s *Store) ClearConsecutive429Count(ctx context.Context, connectionID apid.ID) error {
	return s.r.Del(ctx, consecutive429Key(connectionID)).Err()
}
