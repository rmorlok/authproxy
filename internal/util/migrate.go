package util

import (
	"context"

	"github.com/golang-migrate/migrate/v4"
)

// RunMigrationsUp runs all pending migrations and asks golang-migrate to stop
// at its next safe boundary if ctx is cancelled. golang-migrate does not accept
// a context directly, so callers must bridge cancellation through GracefulStop.
func RunMigrationsUp(ctx context.Context, migrator *migrate.Migrate) error {
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			select {
			case migrator.GracefulStop <- true:
			case <-done:
			}
		case <-done:
		}
	}()

	return migrator.Up()
}
