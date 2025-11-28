package database

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/assert"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestNonces(t *testing.T) {
	t.Run("nonce round trip", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("nonce_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		nonce := uuid.New()

		hasBeenUsed, err := db.HasNonceBeenUsed(ctx, nonce)
		assert.NoError(t, err)
		assert.False(t, hasBeenUsed)

		// This function can be called multiple times with no change
		hasBeenUsed, err = db.HasNonceBeenUsed(ctx, nonce)
		assert.NoError(t, err)
		assert.False(t, hasBeenUsed)

		wasValid, err := db.CheckNonceValidAndMarkUsed(ctx, nonce, now.Add(time.Hour))
		assert.NoError(t, err)
		assert.True(t, wasValid)

		// Nonce should now be used
		hasBeenUsed, err = db.HasNonceBeenUsed(ctx, nonce)
		assert.NoError(t, err)
		assert.True(t, hasBeenUsed)

		// Now nonce should not have been previously valid
		wasValid, err = db.CheckNonceValidAndMarkUsed(ctx, nonce, now.Add(time.Hour))
		assert.NoError(t, err)
		assert.False(t, wasValid)
	})
}
