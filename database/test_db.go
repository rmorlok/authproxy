package database

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"os"
	"path/filepath"
	"time"
)

// MustApplyBlankTestDbConfig applies a test database configuration to the specified config root. The database
// is guaranteed to be blank and migrated. This method uses a temp file so that the database will be eventually
// cleaned up after the process exits. Note that the configuration in the root will be modified for the database
// and populated for the GlobalAESKey if it is not already populated.
//
// Parameters:
// - testName: the name of the test. this can be a blank value but providing it make file names be identifiable by the test that generated them
// - root: the config to apply the database config to. This may be nil, in which case a new config is created. This method will overwrite the existing config.
//
// Returns:
// - the config with information populated for the database. If a config was passed in, the same value is returned with data populated.
// - a database instance configured with the specified root. This database can be used directly, or if the root used again, it will connect to the same database instance.
func MustApplyBlankTestDbConfig(testName string, cfg config.C) (config.C, DB) {
	if testName != "" {
		testName = testName + "-"
	}

	if cfg == nil {
		cfg = config.FromRoot(&config.Root{})
	}

	root := cfg.GetRoot()

	if root == nil {
		panic("No root in config")
	}

	tempFilePath := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("authproxy-tests/db/%s-%d-%s%s.sqlite3", time.Now().Format("2006-01-02T15-04-05"), os.Getpid(), testName, uuid.New().String()),
	)

	dirPath := filepath.Dir(tempFilePath)
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		panic(err)
	}

	_, err = os.Create(tempFilePath)
	if err != nil {
		panic(err)
	}

	root.Database = &config.DatabaseSqlite{
		Provider: config.DatabaseProviderSqlite,
		Path:     tempFilePath,
	}
	if root.SystemAuth.GlobalAESKey == nil {
		root.SystemAuth.GlobalAESKey = &config.KeyDataRandomBytes{}
	}

	db, err := NewConnectionForRoot(root)
	if err != nil {
		panic(err)
	}

	err = db.Migrate(context.Background())
	if err != nil {
		panic(err)
	}

	return cfg, db
}
