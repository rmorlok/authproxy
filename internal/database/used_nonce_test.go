package database

import (
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/stretchr/testify/assert"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestNonces(t *testing.T) {
	t.Run("nonce round trip", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		nonce := apid.New(apid.PrefixActor)

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

	t.Run("nonce expiration", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		fc := clock.NewFakeClock(now)
		ctx := apctx.NewBuilderBackground().WithClock(fc).Build()

		nonce1 := apid.New(apid.PrefixActor)
		nonce2 := apid.New(apid.PrefixActor)

		// Mark nonce1 to expire in 1 hour
		wasValid, err := db.CheckNonceValidAndMarkUsed(ctx, nonce1, now.Add(time.Hour))
		assert.NoError(t, err)
		assert.True(t, wasValid)

		// Mark nonce2 to expire in 2 hours
		wasValid, err = db.CheckNonceValidAndMarkUsed(ctx, nonce2, now.Add(2*time.Hour))
		assert.NoError(t, err)
		assert.True(t, wasValid)

		// Advance time by 1.5 hours
		fc.Step(90 * time.Minute)

		// Delete expired nonces
		err = db.DeleteExpiredNonces(ctx)
		assert.NoError(t, err)

		// Nonce1 should be gone
		hasBeenUsed, err := db.HasNonceBeenUsed(ctx, nonce1)
		assert.NoError(t, err)
		assert.False(t, hasBeenUsed, "nonce1 should have been deleted")

		// Nonce2 should still be there
		hasBeenUsed, err = db.HasNonceBeenUsed(ctx, nonce2)
		assert.NoError(t, err)
		assert.True(t, hasBeenUsed, "nonce2 should still exist")

		// Advance time by another hour (2.5 hours total)
		fc.Step(60 * time.Minute)

		// Delete expired nonces
		err = db.DeleteExpiredNonces(ctx)
		assert.NoError(t, err)

		// Nonce2 should be gone
		hasBeenUsed, err = db.HasNonceBeenUsed(ctx, nonce2)
		assert.NoError(t, err)
		assert.False(t, hasBeenUsed, "nonce2 should have been deleted")
	})
}
