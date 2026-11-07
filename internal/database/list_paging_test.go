package database

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

// keysetPageSize is the page size of keyset tests, and keysetRowCount spans several
// such pages with a partial page at the end.
const (
	keysetPageSize int32 = 10
	keysetRowCount       = 25
)

// asKeys adapts a typed listing to pages of row keys, so one set of scenarios can
// cover every key-ordered listing.
func asKeys[T any](enumerate pagination.EnumerateFunc[T], key func(T) string) pagination.EnumerateFunc[string] {
	return func(ctx context.Context, callback pagination.EnumerateCallback[string]) error {
		return enumerate(ctx, func(page pagination.PageResult[T]) (pagination.KeepGoing, error) {
			keys := make([]string, 0, len(page.Results))
			for _, row := range page.Results {
				keys = append(keys, key(row))
			}
			return callback(pagination.PageResult[string]{
				Results:  keys,
				HasMore:  page.HasMore,
				Cursor:   page.Cursor,
				Position: page.Position,
			})
		})
	}
}

// keysetListing describes one key-ordered listing for the shared scenarios.
type keysetListing struct {
	name string
	// seed creates the i'th row and returns its key.
	seed func(t *testing.T, ctx context.Context, db DB, i int) string
	// remove soft-deletes the row, taking it out of the listing.
	remove func(ctx context.Context, db DB, key string) error
	// enumerate walks without a resume position, as ordinary callers do.
	enumerate func(db DB) pagination.EnumerateFunc[string]
	// resume walks after position.
	resume func(db DB, position string) pagination.EnumerateFunc[string]
	// ordered attempts a resume position together with an explicit order. It is
	// nil for a listing that offers no order.
	ordered func(ctx context.Context, db DB) error
}

// keysetListings returns every listing that supports ResumeAfter.
func keysetListings() []keysetListing {
	connectionKey := func(c Connection) string { return string(c.Id) }
	namespaceKey := func(n Namespace) string { return n.Path }
	encryptionKey := func(k Key) string { return string(k.Id) }
	rateLimitKey := func(r RateLimit) string { return string(r.Id) }
	actorKey := func(a *Actor) string { return string(a.Id) }
	bindingKey := func(b ToolSetBindingListing) string { return string(b.ConnectionId) }
	connectorKey := func(c ConnectorWithDefinition) string { return string(c.Id) }
	generationKey := func(c ConnectorWithDefinition) string { return string(c.DefinitionGenerationId) }

	// generationConnectors maps each seeded generation row id to its connector, so
	// remove can delete the connector that owns it.
	generationConnectors := make(map[string]apid.ID)

	// toolSet is the parent the binding rows share, created with the first row of
	// each fresh database.
	var toolSet *ToolSet
	bindings := func(db DB) ListToolSetBindingsBuilder {
		return db.ListToolSetBindingsBuilder().
			ForToolSetId(toolSet.Id).
			ForToolSetNamespaceMatchers([]string{"root.**"}).
			Limit(keysetPageSize)
	}

	return []keysetListing{
		{
			name: "connections",
			seed: func(t *testing.T, ctx context.Context, db DB, _ int) string {
				id := apid.New(apid.PrefixConnection)
				require.NoError(t, db.CreateConnection(ctx, &Connection{
					Id:                  id,
					Namespace:           "root",
					ConnectorId:         apid.New(apid.PrefixConnector),
					ConnectorGeneration: 1,
					State:               ConnectionStateConfigured,
				}))
				return string(id)
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteConnection(ctx, apid.ID(key))
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(db.ListConnectionsBuilder().Limit(keysetPageSize).Enumerate, connectionKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(db.ListConnectionsBuilder().Limit(keysetPageSize).ResumeAfter(position).Enumerate, connectionKey)
			},
			ordered: func(ctx context.Context, db DB) error {
				return db.ListConnectionsBuilder().
					ResumeAfter("").
					OrderBy(ConnectionOrderByCreatedAt, pagination.OrderByAsc).
					FetchPage(ctx).Error
			},
		},
		{
			name: "namespaces",
			seed: func(t *testing.T, ctx context.Context, db DB, i int) string {
				path := fmt.Sprintf("root.keyset%03d", i)
				require.NoError(t, db.CreateNamespace(ctx, &Namespace{Path: path, State: NamespaceStateActive}))
				return path
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteNamespace(ctx, key)
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(db.ListNamespacesBuilder().Limit(keysetPageSize).Enumerate, namespaceKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(db.ListNamespacesBuilder().Limit(keysetPageSize).ResumeAfter(position).Enumerate, namespaceKey)
			},
			ordered: func(ctx context.Context, db DB) error {
				return db.ListNamespacesBuilder().
					ResumeAfter("").
					OrderBy(NamespaceOrderByPath, pagination.OrderByAsc).
					FetchPage(ctx).Error
			},
		},
		{
			name: "keys",
			seed: func(t *testing.T, ctx context.Context, db DB, _ int) string {
				id := apid.New(apid.PrefixKey)
				require.NoError(t, db.CreateKey(ctx, &Key{Id: id, Namespace: "root", State: KeyStateActive}))
				return string(id)
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteKey(ctx, apid.ID(key))
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(db.ListKeysBuilder().Limit(keysetPageSize).Enumerate, encryptionKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(db.ListKeysBuilder().Limit(keysetPageSize).ResumeAfter(position).Enumerate, encryptionKey)
			},
			ordered: func(ctx context.Context, db DB) error {
				return db.ListKeysBuilder().
					ResumeAfter("").
					OrderBy(KeyOrderByCreatedAt, pagination.OrderByAsc).
					FetchPage(ctx).Error
			},
		},
		{
			name: "rate limits",
			seed: func(t *testing.T, ctx context.Context, db DB, _ int) string {
				id := apid.New(apid.PrefixRateLimit)
				require.NoError(t, db.CreateRateLimit(ctx, &RateLimit{Id: id, Namespace: "root", Definition: validDef()}))
				return string(id)
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteRateLimit(ctx, apid.ID(key))
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(db.ListRateLimitsBuilder().Limit(keysetPageSize).Enumerate, rateLimitKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(db.ListRateLimitsBuilder().Limit(keysetPageSize).ResumeAfter(position).Enumerate, rateLimitKey)
			},
			ordered: func(ctx context.Context, db DB) error {
				return db.ListRateLimitsBuilder().
					ResumeAfter("").
					OrderBy(RateLimitOrderByCreatedAt, pagination.OrderByAsc).
					FetchPage(ctx).Error
			},
		},
		{
			name: "actors",
			seed: func(t *testing.T, ctx context.Context, db DB, _ int) string {
				id := apid.New(apid.PrefixActor)
				require.NoError(t, db.CreateActor(ctx, &Actor{Id: id, Namespace: "root", ExternalId: id.String()}))
				return string(id)
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteActor(ctx, apid.ID(key))
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(db.ListActorsBuilder().Limit(keysetPageSize).Enumerate, actorKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(db.ListActorsBuilder().Limit(keysetPageSize).ResumeAfter(position).Enumerate, actorKey)
			},
			ordered: func(ctx context.Context, db DB) error {
				return db.ListActorsBuilder().
					ResumeAfter("").
					OrderBy(ActorOrderByCreatedAt, pagination.OrderByAsc).
					FetchPage(ctx).Error
			},
		},
		{
			// Each row is a connector with one generation. Deleting the connector takes it
			// out of the listing.
			name: "connectors",
			seed: func(t *testing.T, ctx context.Context, db DB, i int) string {
				connector := testConnectorWithDefinition(apid.New(apid.PrefixConnector), "root", scommon.ResourceName(fmt.Sprintf("keyset-%03d", i)), 1)
				require.NoError(t, db.UpsertConnectorGeneration(ctx, connector))
				return string(connector.Id)
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteConnector(ctx, apid.ID(key))
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(db.ListConnectorsBuilder().Limit(keysetPageSize).Enumerate, connectorKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(db.ListConnectorsBuilder().Limit(keysetPageSize).ResumeAfter(position).Enumerate, connectorKey)
			},
			ordered: func(ctx context.Context, db DB) error {
				return db.ListConnectorsBuilder().
					ResumeAfter("").
					OrderBy(ConnectorOrderByCreatedAt, pagination.OrderByAsc).
					FetchPage(ctx).Error
			},
		},
		{
			// Each row is the only generation of its own connector, keyed by the
			// generation row id. Deleting the connector hides the generation.
			name: "connector generations",
			seed: func(t *testing.T, ctx context.Context, db DB, i int) string {
				connector := testConnectorWithDefinition(apid.New(apid.PrefixConnector), "root", scommon.ResourceName(fmt.Sprintf("keyset-%03d", i)), 1)
				require.NoError(t, db.UpsertConnectorGeneration(ctx, connector))
				generationConnectors[string(connector.DefinitionGenerationId)] = connector.Id
				return string(connector.DefinitionGenerationId)
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteConnector(ctx, generationConnectors[key])
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(db.ListConnectorGenerationsBuilder().Limit(keysetPageSize).Enumerate, generationKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(db.ListConnectorGenerationsBuilder().Limit(keysetPageSize).ResumeAfter(position).Enumerate, generationKey)
			},
			ordered: func(ctx context.Context, db DB) error {
				return db.ListConnectorGenerationsBuilder().
					ResumeAfter("").
					OrderBy(ConnectorGenerationOrderByGeneration, pagination.OrderByAsc).
					FetchPage(ctx).Error
			},
		},
		{
			// One ToolSet's bindings, keyed by connection. Deleting the connection takes
			// its binding out of the listing.
			name: "toolset bindings",
			seed: func(t *testing.T, ctx context.Context, db DB, i int) string {
				if i == 0 {
					toolSet = toolSetForDatabaseTest("root", "keyset-tools")
					require.NoError(t, db.CreateToolSet(ctx, toolSet))

					// Reload for the stored selector revision bindings are created at.
					stored, err := db.GetToolSet(ctx, toolSet.Id)
					require.NoError(t, err)
					toolSet = stored
				}
				id := apid.New(apid.PrefixConnection)
				require.NoError(t, db.CreateConnection(ctx, &Connection{
					Id:                  id,
					Namespace:           "root",
					ConnectorId:         apid.New(apid.PrefixConnector),
					ConnectorGeneration: 1,
					State:               ConnectionStateConfigured,
				}))
				_, err := db.CreateToolSetBinding(ctx, toolSet.Id, id, toolSet.SelectorRevision)
				require.NoError(t, err)
				return string(id)
			},
			remove: func(ctx context.Context, db DB, key string) error {
				return db.DeleteConnection(ctx, apid.ID(key))
			},
			enumerate: func(db DB) pagination.EnumerateFunc[string] {
				return asKeys(bindings(db).Enumerate, bindingKey)
			},
			resume: func(db DB, position string) pagination.EnumerateFunc[string] {
				return asKeys(bindings(db).ResumeAfter(position).Enumerate, bindingKey)
			},
		},
	}
}

// seedKeysetRows creates keysetRowCount rows and returns their keys as a set.
func seedKeysetRows(t *testing.T, ctx context.Context, db DB, listing keysetListing) map[string]bool {
	t.Helper()
	seeded := make(map[string]bool, keysetRowCount)
	for i := 0; i < keysetRowCount; i++ {
		seeded[listing.seed(t, ctx, db, i)] = true
	}
	return seeded
}

// TestEnumerateVisitsEveryRowWhileRemovingThem covers a walk that takes each row out
// of the listing as it goes. An offset walk would skip a page for each page removed.
func TestEnumerateVisitsEveryRowWhileRemovingThem(t *testing.T) {
	for _, listing := range keysetListings() {
		t.Run(listing.name, func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			ctx := reconcileTestCtx()
			seeded := seedKeysetRows(t, ctx, db, listing)

			// Remove every seeded row as soon as its page arrives.
			visits := make(map[string]int)
			err := listing.enumerate(db)(ctx, func(page pagination.PageResult[string]) (pagination.KeepGoing, error) {
				for _, key := range page.Results {
					if seeded[key] {
						visits[key]++
						require.NoError(t, listing.remove(ctx, db, key))
					}
				}
				return pagination.Continue, nil
			})
			require.NoError(t, err)

			// Every row was visited exactly once.
			require.Len(t, visits, keysetRowCount)
			for key, count := range visits {
				require.Equal(t, 1, count, key)
			}
		})
	}
}

// TestResumeAfterContinuesFromPosition covers stopping partway and continuing from
// the reported position with a new listing, as a later task run does.
func TestResumeAfterContinuesFromPosition(t *testing.T) {
	for _, listing := range keysetListings() {
		t.Run(listing.name, func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			ctx := reconcileTestCtx()
			seeded := seedKeysetRows(t, ctx, db, listing)

			// The first run stops after one page and keeps the position.
			visits := make(map[string]int)
			position := ""
			err := listing.resume(db, "")(ctx, func(page pagination.PageResult[string]) (pagination.KeepGoing, error) {
				require.True(t, page.HasMore)
				require.Equal(t, page.Results[len(page.Results)-1], page.Position)
				for _, key := range page.Results {
					visits[key]++
				}
				position = page.Position
				return pagination.Stop, nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, position)

			// The row at the position is removed, as a concurrent delete might, and the
			// walk must still continue after it.
			if seeded[position] {
				require.NoError(t, listing.remove(ctx, db, position))
			}

			// The second run sees everything after the position, including its last page's
			// position, and nothing the first run already saw.
			lastPosition := ""
			err = listing.resume(db, position)(ctx, func(page pagination.PageResult[string]) (pagination.KeepGoing, error) {
				for _, key := range page.Results {
					visits[key]++
				}
				lastPosition = page.Position
				return pagination.Continue, nil
			})
			require.NoError(t, err)
			require.NotEmpty(t, lastPosition)

			for key := range seeded {
				require.Equal(t, 1, visits[key], key)
			}
		})
	}
}

// TestResumeAfterRejectsExplicitOrder covers the invalid combination of options.
func TestResumeAfterRejectsExplicitOrder(t *testing.T) {
	for _, listing := range keysetListings() {
		t.Run(listing.name, func(t *testing.T) {
			if listing.ordered == nil {
				t.Skip("listing offers no order")
			}
			_, db := MustApplyBlankTestDbConfig(t, nil)

			// A key position means nothing in another order, so the listing refuses to run.
			require.ErrorIs(t, listing.ordered(reconcileTestCtx(), db), ErrInvalidListQuery)
		})
	}
}

// TestKeysetListingModes covers how key order interacts with the offset paging that
// API callers still use.
func TestKeysetListingModes(t *testing.T) {
	t.Run("FetchPage stays offset paged", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := reconcileTestCtx()
		seedKeysetRows(t, ctx, db, keysetListings()[0])

		// Only key-ordered listings report a position.
		page := db.ListConnectionsBuilder().Limit(keysetPageSize).FetchPage(ctx)
		require.NoError(t, page.Error)
		require.True(t, page.HasMore)
		require.Empty(t, page.Position)
	})

	t.Run("cursor from an offset page keeps offset paging under Enumerate", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := reconcileTestCtx()
		seedKeysetRows(t, ctx, db, keysetListings()[0])

		// Take the first page by offset, in the listing's stable default order.
		first := db.ListConnectionsBuilder().Limit(keysetPageSize).FetchPage(ctx)
		require.NoError(t, first.Error)

		visits := make(map[apid.ID]int)
		for _, connection := range first.Results {
			visits[connection.Id]++
		}

		// Enumerating the cursor continues past the first page without repeating it.
		executor, err := db.ListConnectionsFromCursor(ctx, first.Cursor)
		require.NoError(t, err)
		err = executor.Enumerate(ctx, func(page pagination.PageResult[Connection]) (pagination.KeepGoing, error) {
			require.Empty(t, page.Position)
			for _, connection := range page.Results {
				visits[connection.Id]++
			}
			return pagination.Continue, nil
		})
		require.NoError(t, err)

		require.Len(t, visits, keysetRowCount)
		for id, count := range visits {
			require.Equal(t, 1, count, id)
		}
	})

	t.Run("cursor from a key-ordered page stays key ordered", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		ctx := reconcileTestCtx()
		seedKeysetRows(t, ctx, db, keysetListings()[0])

		// Take the first page in key order.
		first := db.ListConnectionsBuilder().Limit(keysetPageSize).ResumeAfter("").FetchPage(ctx)
		require.NoError(t, first.Error)
		require.NotEmpty(t, first.Position)

		// The cursor's next page reports a position and repeats nothing.
		executor, err := db.ListConnectionsFromCursor(ctx, first.Cursor)
		require.NoError(t, err)
		second := executor.FetchPage(ctx)
		require.NoError(t, second.Error)
		require.NotEmpty(t, second.Position)
		for _, connection := range second.Results {
			require.NotContains(t, first.Results, connection)
		}
	})

	t.Run("explicit order is kept under Enumerate", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC)
		earlier := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()
		later := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now.Add(time.Minute))).Build()

		// Two keys are created a minute apart.
		first := apid.New(apid.PrefixKey)
		second := apid.New(apid.PrefixKey)
		require.NoError(t, db.CreateKey(earlier, &Key{Id: first, Namespace: "root", State: KeyStateActive}))
		require.NoError(t, db.CreateKey(later, &Key{Id: second, Namespace: "root", State: KeyStateActive}))

		// An ordered listing has no position and follows the requested order.
		var ids []apid.ID
		err := db.ListKeysBuilder().
			OrderBy(KeyOrderByCreatedAt, pagination.OrderByDesc).
			Enumerate(later, func(page pagination.PageResult[Key]) (pagination.KeepGoing, error) {
				require.Empty(t, page.Position)
				for _, key := range page.Results {
					if key.Id == first || key.Id == second {
						ids = append(ids, key.Id)
					}
				}
				return pagination.Continue, nil
			})
		require.NoError(t, err)
		require.Equal(t, []apid.ID{second, first}, ids)
	})
}

// TestListPaging covers listPaging's mode selection, the SQL it adds, and how
// nextPage advances, without a database.
func TestListPaging(t *testing.T) {
	// toSql renders the paging that p adds to a bare select.
	toSql := func(t *testing.T, p *listPaging[ConnectionOrderByField], defaultOrder ...string) (string, []any) {
		sqlStr, args, err := p.apply(sq.Select("*").From("t"), "id", defaultOrder...).ToSql()
		require.NoError(t, err)
		return sqlStr, args
	}
	// key reads a row's key; the rows in these tests are their own keys.
	key := func(s string) string { return s }

	t.Run("offset mode uses the default order and page size", func(t *testing.T) {
		var p listPaging[ConnectionOrderByField]

		// An unset limit becomes the default, fetched one row over.
		sqlStr, _ := toSql(t, &p, "created_at ASC", "id ASC")
		require.Equal(t, "SELECT * FROM t ORDER BY created_at ASC, id ASC LIMIT 101 OFFSET 0", sqlStr)
		require.Equal(t, uint64(defaultListPageSize), p.LimitVal)
	})

	t.Run("offset mode uses the caller's order", func(t *testing.T) {
		var p listPaging[ConnectionOrderByField]
		p.setLimit(2)
		p.setOrder(ConnectionOrderByCreatedAt, pagination.OrderByDesc)

		// The caller's order replaces the default.
		sqlStr, _ := toSql(t, &p, "created_at ASC", "id ASC")
		require.Equal(t, "SELECT * FROM t ORDER BY created_at DESC LIMIT 3 OFFSET 0", sqlStr)
	})

	t.Run("applyOrdered maps the caller's order to the listing's clauses", func(t *testing.T) {
		var p listPaging[ConnectionOrderByField]
		p.setLimit(2)
		p.setOrder(ConnectionOrderByCreatedAt, pagination.OrderByDesc)
		clauses := func(field ConnectionOrderByField, direction string) []string {
			return []string{"x." + string(field) + " " + direction, "x.id " + direction}
		}

		// In offset mode the mapped clauses, tie-breaker included, replace the field name.
		sqlStr, _, err := p.applyOrdered(sq.Select("*").From("t x"), "x.id", clauses).ToSql()
		require.NoError(t, err)
		require.Equal(t, "SELECT * FROM t x ORDER BY x.created_at DESC, x.id DESC LIMIT 3 OFFSET 0", sqlStr)

		// In key mode the mapping is unused, so it pages by the qualified key.
		var keyed listPaging[ConnectionOrderByField]
		keyed.setLimit(2)
		keyed.resumeAfter("b")
		sqlStr, args, err := keyed.applyOrdered(sq.Select("*").From("t x"), "x.id", clauses).ToSql()
		require.NoError(t, err)
		require.Equal(t, "SELECT * FROM t x WHERE x.id > ? ORDER BY x.id ASC LIMIT 3", sqlStr)
		require.Equal(t, []any{"b"}, args)
	})

	t.Run("offset mode advances by the rows returned", func(t *testing.T) {
		var p listPaging[ConnectionOrderByField]
		p.setLimit(2)

		// A full page plus the extra row reports more and moves past the page only.
		page, hasMore, position := nextPage(&p, []string{"a", "b", "c"}, key)
		require.Equal(t, []string{"a", "b"}, page)
		require.True(t, hasMore)
		require.Empty(t, position)
		require.Equal(t, uint64(2), p.Offset)

		// The next query skips the rows already returned.
		sqlStr, _ := toSql(t, &p)
		require.Equal(t, "SELECT * FROM t LIMIT 3 OFFSET 2", sqlStr)

		// A short final page reports no more.
		page, hasMore, _ = nextPage(&p, []string{"c"}, key)
		require.Equal(t, []string{"c"}, page)
		require.False(t, hasMore)
		require.Equal(t, uint64(3), p.Offset)
	})

	t.Run("key mode pages after the position", func(t *testing.T) {
		var p listPaging[ConnectionOrderByField]
		p.setLimit(2)
		p.resumeAfter("")

		// The first query starts at the beginning in key order, ignoring the default.
		sqlStr, args := toSql(t, &p, "created_at ASC", "id ASC")
		require.Equal(t, "SELECT * FROM t ORDER BY id ASC LIMIT 3", sqlStr)
		require.Empty(t, args)

		// The page moves the position to its last row rather than the offset.
		page, hasMore, position := nextPage(&p, []string{"a", "b", "c"}, key)
		require.Equal(t, []string{"a", "b"}, page)
		require.True(t, hasMore)
		require.Equal(t, "b", position)
		require.Zero(t, p.Offset)

		// The next query restricts to rows after the position.
		sqlStr, args = toSql(t, &p)
		require.Equal(t, "SELECT * FROM t WHERE id > ? ORDER BY id ASC LIMIT 3", sqlStr)
		require.Equal(t, []any{"b"}, args)

		// An empty page keeps reporting the last position.
		_, hasMore, position = nextPage(&p, []string(nil), key)
		require.False(t, hasMore)
		require.Equal(t, "b", position)
	})

	t.Run("startEnumerate picks key mode only for a fresh unordered walk", func(t *testing.T) {
		// A fresh listing with no order switches to key mode.
		var fresh listPaging[ConnectionOrderByField]
		fresh.startEnumerate()
		require.True(t, fresh.Keyset)

		// A caller-chosen order is kept.
		var ordered listPaging[ConnectionOrderByField]
		ordered.setOrder(ConnectionOrderByCreatedAt, pagination.OrderByAsc)
		ordered.startEnumerate()
		require.False(t, ordered.Keyset)

		// A listing partway through its pages by offset stays offset paged.
		resumed := listPaging[ConnectionOrderByField]{Offset: 10}
		resumed.startEnumerate()
		require.False(t, resumed.Keyset)
	})

	t.Run("validate rejects key mode with a caller-chosen order", func(t *testing.T) {
		var p listPaging[ConnectionOrderByField]
		p.setOrder(ConnectionOrderByCreatedAt, pagination.OrderByAsc)
		require.NoError(t, p.validate())

		// Adding a position to an ordered listing is rejected.
		p.resumeAfter("b")
		require.ErrorIs(t, p.validate(), ErrInvalidListQuery)
	})

	t.Run("embedded fields encode at the top level of the cursor", func(t *testing.T) {
		var filters listConnectionsFilters
		filters.setLimit(5)
		filters.resumeAfter("b")

		// The paging fields sit beside the listing's own filters, not under a nested key.
		encoded, err := json.Marshal(&filters)
		require.NoError(t, err)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(encoded, &fields))
		require.Equal(t, float64(5), fields["limit"])
		require.Equal(t, true, fields["keyset"])
		require.Equal(t, "b", fields["afterKey"])
		require.NotContains(t, fields, "listPaging")

		// Decoding restores the same paging state.
		var decoded listConnectionsFilters
		require.NoError(t, json.Unmarshal(encoded, &decoded))
		require.Equal(t, filters.listPaging, decoded.listPaging)
	})
}
