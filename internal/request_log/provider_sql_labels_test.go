package request_log

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

func newTestSqliteDb(t *testing.T) (*sql.DB, string) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "request_log_test_*.db")
	require.NoError(t, err)
	tmpFile.Close()

	path := tmpFile.Name()
	t.Cleanup(func() { os.Remove(path) })

	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on&_journal_mode=WAL")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return db, path
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestSqlRecordStoreAndRetriever(t *testing.T) (*sqlRecordStore, *sqlRecordRetriever) {
	t.Helper()

	db, path := newTestSqliteDb(t)
	logger := newTestLogger()

	cfg := &sconfig.Database{
		InnerVal: &sconfig.DatabaseSqlite{
			Path: path,
		},
	}

	store := &sqlRecordStore{
		db:                db,
		uri:               cfg.GetUri(),
		provider:          cfg.GetProvider(),
		cfg:               cfg,
		logger:            logger,
		placeholderFormat: sq.Question,
	}

	err := store.Migrate(context.Background())
	require.NoError(t, err)

	cursorEnc := pagination.NewDefaultCursorEncryptor([]byte("0123456789abcdef0123456789abcdef"))

	retriever := &sqlRecordRetriever{
		db:                db,
		cursorEncryptor:   cursorEnc,
		logger:            logger,
		provider:          sconfig.DatabaseProviderSqlite,
		placeholderFormat: sq.Question,
	}

	return store, retriever
}

func makeTestLogRecord(namespace string, labels database.Labels) *LogRecord {
	return &LogRecord{
		RequestId:          apid.New(apid.PrefixRequestLog),
		Namespace:          namespace,
		Type:               httpf.RequestTypeProxy,
		Timestamp:          time.Now().UTC().Truncate(time.Millisecond),
		MillisecondDuration: MillisecondDuration(100 * time.Millisecond),
		Method:             "GET",
		Host:               "example.com",
		Scheme:             "https",
		Path:               "/api/test",
		ResponseStatusCode: 200,
		Labels:             labels,
	}
}

func TestSqlStoreAndRetrieveLabels(t *testing.T) {
	store, retriever := newTestSqlRecordStoreAndRetriever(t)
	ctx := context.Background()

	t.Run("store and retrieve record with labels", func(t *testing.T) {
		labels := database.Labels{"env": "prod", "team": "api"}
		record := makeTestLogRecord("root", labels)

		err := store.StoreRecord(ctx, record)
		require.NoError(t, err)

		got, err := retriever.GetRecord(ctx, record.RequestId)
		require.NoError(t, err)
		require.Equal(t, record.RequestId, got.RequestId)
		require.Equal(t, database.Labels{"env": "prod", "team": "api"}, got.Labels)
	})

	t.Run("store and retrieve record with nil labels", func(t *testing.T) {
		record := makeTestLogRecord("root", nil)

		err := store.StoreRecord(ctx, record)
		require.NoError(t, err)

		got, err := retriever.GetRecord(ctx, record.RequestId)
		require.NoError(t, err)
		require.Equal(t, record.RequestId, got.RequestId)
		// nil labels stored as '{}' default, scanned back as empty map or nil
		require.True(t, len(got.Labels) == 0)
	})

	t.Run("store and retrieve record with empty labels", func(t *testing.T) {
		record := makeTestLogRecord("root", database.Labels{})

		err := store.StoreRecord(ctx, record)
		require.NoError(t, err)

		got, err := retriever.GetRecord(ctx, record.RequestId)
		require.NoError(t, err)
		require.True(t, len(got.Labels) == 0)
	})

	t.Run("store multiple records with different labels", func(t *testing.T) {
		r1 := makeTestLogRecord("root", database.Labels{"env": "prod"})
		r2 := makeTestLogRecord("root", database.Labels{"env": "staging", "region": "us-east"})
		r3 := makeTestLogRecord("root", nil)

		err := store.StoreRecords(ctx, []*LogRecord{r1, r2, r3})
		require.NoError(t, err)

		got1, err := retriever.GetRecord(ctx, r1.RequestId)
		require.NoError(t, err)
		require.Equal(t, database.Labels{"env": "prod"}, got1.Labels)

		got2, err := retriever.GetRecord(ctx, r2.RequestId)
		require.NoError(t, err)
		require.Equal(t, database.Labels{"env": "staging", "region": "us-east"}, got2.Labels)

		got3, err := retriever.GetRecord(ctx, r3.RequestId)
		require.NoError(t, err)
		require.True(t, len(got3.Labels) == 0)
	})
}

func TestSqlListWithLabelSelector(t *testing.T) {
	store, retriever := newTestSqlRecordStoreAndRetriever(t)
	ctx := context.Background()

	// Insert test data
	r1 := makeTestLogRecord("root", database.Labels{"env": "prod", "team": "api"})
	r2 := makeTestLogRecord("root", database.Labels{"env": "staging", "team": "api"})
	r3 := makeTestLogRecord("root", database.Labels{"env": "prod", "team": "web"})
	r4 := makeTestLogRecord("root", database.Labels{"team": "api"})
	r5 := makeTestLogRecord("root", nil)

	err := store.StoreRecords(ctx, []*LogRecord{r1, r2, r3, r4, r5})
	require.NoError(t, err)

	t.Run("filter by label equality", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("env=prod")
		require.NoError(t, err)

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 2)

		ids := map[apid.ID]bool{}
		for _, r := range result.Results {
			ids[r.RequestId] = true
		}
		require.True(t, ids[r1.RequestId])
		require.True(t, ids[r3.RequestId])
	})

	t.Run("filter by multiple label equalities", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("env=prod,team=api")
		require.NoError(t, err)

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 1)
		require.Equal(t, r1.RequestId, result.Results[0].RequestId)
	})

	t.Run("filter by label exists", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("env")
		require.NoError(t, err)

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 3) // r1, r2, r3

		ids := map[apid.ID]bool{}
		for _, r := range result.Results {
			ids[r.RequestId] = true
		}
		require.True(t, ids[r1.RequestId])
		require.True(t, ids[r2.RequestId])
		require.True(t, ids[r3.RequestId])
	})

	t.Run("filter by label not exists", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("!env")
		require.NoError(t, err)

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 2) // r4, r5

		ids := map[apid.ID]bool{}
		for _, r := range result.Results {
			ids[r.RequestId] = true
		}
		require.True(t, ids[r4.RequestId])
		require.True(t, ids[r5.RequestId])
	})

	t.Run("filter by label not equal", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("env!=prod")
		require.NoError(t, err)

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		// r2 has env=staging, r4 has no env, r5 has no env
		require.Len(t, result.Results, 3)

		ids := map[apid.ID]bool{}
		for _, r := range result.Results {
			ids[r.RequestId] = true
		}
		require.True(t, ids[r2.RequestId])
		require.True(t, ids[r4.RequestId])
		require.True(t, ids[r5.RequestId])
	})

	t.Run("no results for non-matching selector", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("env=nonexistent")
		require.NoError(t, err)

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 0)
	})

	t.Run("invalid label selector returns error", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		_, err := b.WithLabelSelector("invalid key with spaces=value")
		require.Error(t, err)
	})

	t.Run("label selector combined with other filters", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("team=api")
		require.NoError(t, err)
		b = b.WithNamespaceMatcher("root")

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 3) // r1, r2, r4

		ids := map[apid.ID]bool{}
		for _, r := range result.Results {
			ids[r.RequestId] = true
		}
		require.True(t, ids[r1.RequestId])
		require.True(t, ids[r2.RequestId])
		require.True(t, ids[r4.RequestId])
	})

	t.Run("labels are returned in list results", func(t *testing.T) {
		b := retriever.NewListRequestsBuilder()
		b, err := b.WithLabelSelector("env=prod,team=api")
		require.NoError(t, err)

		result := b.FetchPage(ctx)
		require.NoError(t, result.Error)
		require.Len(t, result.Results, 1)
		require.Equal(t, database.Labels{"env": "prod", "team": "api"}, result.Results[0].Labels)
	})
}
