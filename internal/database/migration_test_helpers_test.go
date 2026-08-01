package database

import (
	"fmt"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"
)

func migrateDatabaseToVersion(t *testing.T, service *service, version uint) {
	t.Helper()

	source, err := iofs.New(
		migrationsFs,
		fmt.Sprintf("migrations/%s", service.cfg.GetProvider()),
	)
	require.NoError(t, err)

	migrator, err := migrate.NewWithSourceInstance("iofs", source, service.cfg.GetUri())
	require.NoError(t, err)
	defer func() {
		sourceErr, databaseErr := migrator.Close()
		require.NoError(t, sourceErr)
		require.NoError(t, databaseErr)
	}()

	require.NoError(t, migrator.Migrate(version))
}
