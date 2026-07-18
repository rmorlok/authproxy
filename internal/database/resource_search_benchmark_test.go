package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
)

// BenchmarkSearchResourcesAt100K is an opt-in load scenario for the bounded
// live-search design. Normal unit tests do not make wall-clock assertions; run
// this benchmark explicitly for each database provider when evaluating search
// performance or changing the provider-specific SQL.
func BenchmarkSearchResourcesAt100K(b *testing.B) {
	b.StopTimer()
	cfg, db, raw := MustApplyBlankTestDbConfigRaw(b, nil)
	require.NoError(b, db.EnsureNamespaceByPath(b.Context(), "root.search-load"))

	placeholders := "?, ?, ?, ?, CURRENT_TIMESTAMP, ?"
	if cfg.GetRoot().Database.GetProvider() == config.DatabaseProviderPostgres {
		placeholders = "$1, $2, $3, $4, CURRENT_TIMESTAMP, $5"
	}
	tx, err := raw.BeginTx(b.Context(), nil)
	require.NoError(b, err)
	statement, err := tx.PrepareContext(b.Context(), fmt.Sprintf(
		"INSERT INTO actors (id, namespace, labels, external_id, created_at, updated_at) VALUES (%s)",
		placeholders,
	))
	require.NoError(b, err)
	for i := 0; i < 100_000; i++ {
		label := fmt.Sprintf(`{"name":"resource-%06d"}`, i)
		if i == 99_999 {
			label = `{"name":"bounded-search-needle"}`
		}
		_, err = statement.ExecContext(
			b.Context(),
			fmt.Sprintf("act_searchload%013d", i),
			"root.search-load",
			label,
			fmt.Sprintf("search-load-%06d", i),
			time.Unix(int64(i), 0).UTC(),
		)
		require.NoError(b, err)
	}
	require.NoError(b, statement.Close())
	require.NoError(b, tx.Commit())

	benchmarks := []struct {
		name         string
		query        string
		allowTimeout bool
	}{
		{name: "recent-seed"},
		{name: "generic-substring", query: "needle", allowTimeout: true},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithTimeout(b.Context(), 750*time.Millisecond)
				result, err := db.SearchResources(ctx, SearchResourcesParams{
					ResourceType:      SearchResourceTypeActor,
					Query:             benchmark.query,
					NamespaceMatchers: []string{"root.search-load.**"},
					Limit:             50,
				})
				cancel()
				isTimeout := errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
				if err != nil && (!benchmark.allowTimeout || !isTimeout) {
					b.Fatal(err)
				}
				if err == nil && len(result.Items) > 50 {
					b.Fatalf("search returned %d items, limit was 50", len(result.Items))
				}
				if benchmark.query == "" && err == nil && (len(result.Items) != 50 || !result.Truncated) {
					b.Fatalf("seed returned %d items with truncated=%t; expected 50 bounded items", len(result.Items), result.Truncated)
				}
			}
		})
	}
}
