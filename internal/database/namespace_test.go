package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestNamespaces(t *testing.T) {
	t.Run("advanceNamespaceState", func(t *testing.T) {
		tests := []struct {
			cur  NamespaceState
			next NamespaceState
			want NamespaceState
		}{
			{
				cur:  NamespaceStateActive,
				next: NamespaceStateDestroying,
				want: NamespaceStateDestroying,
			},
			{
				cur:  NamespaceStateDestroying,
				next: NamespaceStateDestroyed,
				want: NamespaceStateDestroyed,
			},
			{
				cur:  NamespaceStateDestroyed,
				next: NamespaceStateActive,
				want: NamespaceStateDestroyed,
			},
			{
				cur:  NamespaceState(""),
				next: NamespaceStateActive,
				want: NamespaceStateActive,
			},
		}
		for _, test := range tests {
			t.Run(fmt.Sprintf("'%s' -> '%s'", test.cur, test.next), func(t *testing.T) {
				got := advanceNamespaceState(test.cur, test.next)
				require.Equal(t, test.want, got)
			})
		}
	})
	t.Run("basic", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		_, err := rawDb.Exec(`DELETE FROM namespaces`)
		require.NoError(t, err)

		sql := `
INSERT INTO namespaces 
(path,                   depth, state,       created_at,            updated_at,            deleted_at) VALUES 
('root',                 0,     'active',    '2023-10-01 00:00:00', '2023-11-01 00:00:00', null),
('root.prod',            1,     'active',    '2023-10-02 00:00:00', '2023-11-02 00:00:00', null),
('root.prod.12345',      2,     'active',    '2023-10-04 00:00:00', '2023-11-03 00:00:00', null),
('root.prod.54321',      2,     'active',    '2023-10-03 00:00:00', '2023-11-04 00:00:00', null),
('root.prod.99999',      2,     'destroyed', '2023-10-03 03:00:00', '2023-11-04 01:00:00', null),
('root.prod-like',       1,     'active',    '2023-10-02 00:00:00', '2023-11-02 00:00:00', null),
('root.prod-like.77777', 2,     'destroyed', '2023-10-03 03:00:00', '2023-11-04 02:00:00', null),
('root.prod.88888',      2,     'destroyed', '2023-10-03 04:00:00', '2023-11-04 04:00:00', '2023-11-04 05:00:00'),
('root.dev',             1,     'active',    '2023-10-02 01:00:00', '2023-11-05 00:00:00', null)
`
		_, err = rawDb.Exec(sql)
		require.NoError(t, err)

		ns, err := db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, sconfig.RootNamespace, ns.Path)
		require.Equal(t, NamespaceStateActive, ns.State)

		// Namespace doesn't exist
		ns, err = db.GetNamespace(ctx, "does-not-exist")
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, ns)

		pr := db.ListNamespacesBuilder().
			ForPathPrefix("root.prod").
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Path, "root.prod.12345")
		require.Equal(t, pr.Results[1].Path, "root.prod.99999")
		require.Equal(t, pr.Results[2].Path, "root.prod.54321")
		require.Equal(t, pr.Results[3].Path, "root.prod")

		pr = db.ListNamespacesBuilder().
			ForDepth(2).
			OrderBy(NamespaceOrderByUpdatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Path, "root.prod-like.77777")
		require.Equal(t, pr.Results[1].Path, "root.prod.99999")
		require.Equal(t, pr.Results[2].Path, "root.prod.54321")
		require.Equal(t, pr.Results[3].Path, "root.prod.12345")

		pr = db.ListNamespacesBuilder().
			ForChildrenOf("root.prod").
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 3)
		require.Equal(t, pr.Results[0].Path, "root.prod.12345")
		require.Equal(t, pr.Results[1].Path, "root.prod.99999")
		require.Equal(t, pr.Results[2].Path, "root.prod.54321")

		pr = db.ListNamespacesBuilder().
			ForNamespaceMatcher("root.prod").
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 1)
		require.Equal(t, pr.Results[0].Path, "root.prod")

		pr = db.ListNamespacesBuilder().
			ForNamespaceMatcher("root.prod.**").
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Path, "root.prod.12345")
		require.Equal(t, pr.Results[1].Path, "root.prod.99999")
		require.Equal(t, pr.Results[2].Path, "root.prod.54321")
		require.Equal(t, pr.Results[3].Path, "root.prod")

		// Invalid matcher
		pr = db.ListNamespacesBuilder().
			ForNamespaceMatcher("root.prod**").
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.Error(t, pr.Error)

		pr = db.ListNamespacesBuilder().
			ForPathPrefix("root.prod").
			ForState(NamespaceStateDestroyed).
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 1)
		require.Equal(t, pr.Results[0].Path, "root.prod.99999")

		pr = db.ListNamespacesBuilder().
			ForPathPrefix("root.prod").
			ForState(NamespaceStateDestroyed).
			IncludeDeleted().
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 2)
		require.Equal(t, pr.Results[0].Path, "root.prod.88888")
		require.Equal(t, pr.Results[1].Path, "root.prod.99999")

		count := 0
		err = db.
			ListNamespacesBuilder().
			Enumerate(ctx, func(page pagination.PageResult[Namespace]) (bool, error) {
				count += len(page.Results)
				return true, nil
			})
		require.NoError(t, err)
		require.Equal(t, count, 8)
	})
	t.Run("CreateNamespace", func(t *testing.T) {
		t.Run("creates a new namespace", func(t *testing.T) {
			// Setup
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := rawDb.Exec(`DELETE FROM namespaces`)
			require.NoError(t, err)

			ns := &Namespace{
				Path:  sconfig.RootNamespace,
				State: NamespaceStateActive,
			}

			// Test
			err = db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.NoError(t, err)
			require.NotNil(t, saveNs)
			require.Equal(t, ns.Path, saveNs.Path)
		})

		t.Run("creates a child namespace", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  "root.child",
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.NoError(t, err)
			require.NotNil(t, saveNs)
			require.Equal(t, ns.Path, saveNs.Path)
		})

		t.Run("refuses to create a namespace with invalid name", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  "root.#invalid#",
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.Error(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, saveNs)
		})

		t.Run("refuses to create an un-rooted namespace", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  "child",
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.Error(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, saveNs)
		})

		t.Run("refuses to create a namespace where parent doesn't exist", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  "root.does-not-exist.child",
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.Error(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, saveNs)
		})

		t.Run("refuses to create a namespace where parent is deleted", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  "root.parent",
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			err = db.DeleteNamespace(ctx, ns.Path)
			require.NoError(t, err)

			ns = &Namespace{
				Path:  "root.does-not-exist.child",
				State: NamespaceStateActive,
			}

			err = db.CreateNamespace(ctx, ns)
			require.Error(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, saveNs)
		})
	})
	t.Run("GetNamespace", func(t *testing.T) {
		// Setup
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		retrieved, err := db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, sconfig.RootNamespace, retrieved.Path)
		require.Equal(t, NamespaceStateActive, retrieved.State)
	})
	t.Run("DeleteNamespace", func(t *testing.T) {
		// Setup
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		retrieved, err := db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, sconfig.RootNamespace, retrieved.Path)

		err = db.DeleteNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)

		retrieved, err = db.GetNamespace(ctx, sconfig.RootNamespace)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, retrieved)
	})
	t.Run("SetNamespaceState", func(t *testing.T) {
		// Setup
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		retrieved, err := db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, sconfig.RootNamespace, retrieved.Path)
		require.Equal(t, NamespaceStateActive, retrieved.State)

		err = db.SetNamespaceState(ctx, sconfig.RootNamespace, NamespaceStateDestroying)
		require.NoError(t, err)

		retrieved, err = db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, sconfig.RootNamespace, sconfig.RootNamespace)
		require.Equal(t, NamespaceStateDestroying, retrieved.State)
	})
	t.Run("normalize", func(t *testing.T) {
		val := Namespace{
			Path:  "root.prod.12345",
			State: NamespaceStateDestroying,
		}

		val.normalize()
		require.Equal(t, "root.prod.12345", val.Path)
		require.Equal(t, uint64(2), val.depth)
		require.Equal(t, NamespaceStateDestroying, val.State)

		val = Namespace{
			Path: "root",
		}

		val.normalize()
		require.Equal(t, "root", val.Path)
		require.Equal(t, uint64(0), val.depth)
		require.Equal(t, NamespaceStateActive, val.State)
	})
	t.Run("Validate", func(t *testing.T) {
		val := Namespace{
			Path:  "root.prod.12345",
			State: NamespaceStateDestroying,
		}

		err := val.Validate()
		require.NoError(t, err)

		val = Namespace{
			Path:  "",
			State: NamespaceStateDestroying,
		}

		err = val.Validate()
		require.Error(t, err)

		val = Namespace{
			Path: "root",
		}

		err = val.Validate()
		require.Error(t, err)
	})
	t.Run("ForNamespaceMatchers", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		_, err := rawDb.Exec(`DELETE FROM namespaces`)
		require.NoError(t, err)

		sql := `
INSERT INTO namespaces
(path,                   depth, state,       created_at,            updated_at,            deleted_at) VALUES
('root',                 0,     'active',    '2023-10-01 00:00:00', '2023-11-01 00:00:00', null),
('root.prod',            1,     'active',    '2023-10-02 00:00:00', '2023-11-02 00:00:00', null),
('root.prod.tenant1',    2,     'active',    '2023-10-03 00:00:00', '2023-11-03 00:00:00', null),
('root.prod.tenant2',    2,     'active',    '2023-10-04 00:00:00', '2023-11-04 00:00:00', null),
('root.staging',         1,     'active',    '2023-10-05 00:00:00', '2023-11-05 00:00:00', null),
('root.staging.tenant1', 2,     'active',    '2023-10-06 00:00:00', '2023-11-06 00:00:00', null),
('root.dev',             1,     'active',    '2023-10-07 00:00:00', '2023-11-07 00:00:00', null)
`
		_, err = rawDb.Exec(sql)
		require.NoError(t, err)

		t.Run("empty matchers returns all", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{}).
				OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 7)
		})

		t.Run("single exact matcher", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{"root.prod"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 1)
			require.Equal(t, "root.prod", pr.Results[0].Path)
		})

		t.Run("single wildcard matcher", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{"root.prod.**"}).
				OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 3)
			require.Equal(t, "root.prod", pr.Results[0].Path)
			require.Equal(t, "root.prod.tenant1", pr.Results[1].Path)
			require.Equal(t, "root.prod.tenant2", pr.Results[2].Path)
		})

		t.Run("multiple exact matchers (OR logic)", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{"root.prod", "root.staging", "root.dev"}).
				OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 3)
			paths := []string{pr.Results[0].Path, pr.Results[1].Path, pr.Results[2].Path}
			require.Contains(t, paths, "root.prod")
			require.Contains(t, paths, "root.staging")
			require.Contains(t, paths, "root.dev")
		})

		t.Run("multiple wildcard matchers (OR logic)", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{"root.prod.**", "root.staging.**"}).
				OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 5)
			paths := make([]string, len(pr.Results))
			for i, r := range pr.Results {
				paths[i] = r.Path
			}
			require.Contains(t, paths, "root.prod")
			require.Contains(t, paths, "root.prod.tenant1")
			require.Contains(t, paths, "root.prod.tenant2")
			require.Contains(t, paths, "root.staging")
			require.Contains(t, paths, "root.staging.tenant1")
		})

		t.Run("mixed exact and wildcard matchers", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{"root.prod.**", "root.dev"}).
				OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByAsc).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 4)
			paths := make([]string, len(pr.Results))
			for i, r := range pr.Results {
				paths[i] = r.Path
			}
			require.Contains(t, paths, "root.prod")
			require.Contains(t, paths, "root.prod.tenant1")
			require.Contains(t, paths, "root.prod.tenant2")
			require.Contains(t, paths, "root.dev")
		})

		t.Run("no matching namespaces", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{"root.nonexistent"}).
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 0)
		})

		t.Run("invalid matcher returns error", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForNamespaceMatchers([]string{"root.prod", "invalid**"}).
				FetchPage(ctx)
			require.Error(t, pr.Error)
		})
	})
	t.Run("EnsureNamespaceByPath", func(t *testing.T) {
		t.Run("does not duplicate namespaces", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.EnsureNamespaceByPath(ctx, "root")
			require.NoError(t, err)
			err = db.EnsureNamespaceByPath(ctx, "root.prod")
			require.NoError(t, err)
			err = db.EnsureNamespaceByPath(ctx, "root.prod.12345")
			require.NoError(t, err)
			err = db.EnsureNamespaceByPath(ctx, "root.prod.12345")
			require.NoError(t, err)

			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM namespaces WHERE path = 'root'"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM namespaces WHERE path = 'root.prod'"))
			require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM namespaces WHERE path = 'root.prod.12345'"))
		})

		tests := []struct {
			name        string
			setupSQL    string
			path        string
			expectError bool
		}{
			{
				name:        "creates new namespace successfully",
				path:        "root.newnamespace",
				expectError: false,
			},
			{
				name:        "recursively creates child namespaces",
				path:        "root.child.grandchild.greatgrandchild",
				expectError: false,
			},
			{
				name:        "handles existing active namespace",
				setupSQL:    "INSERT INTO namespaces (path, depth, state, created_at, updated_at) VALUES ('root.active', 0, 'active', '2023-10-01', '2023-10-01');",
				path:        "root.active",
				expectError: false,
			},
			{
				name:        "returns error when namespace is inactive",
				setupSQL:    "INSERT INTO namespaces (path, depth, state, created_at, updated_at) VALUES ('root.inactive', 0, 'destroyed', '2023-10-01', '2023-10-01');",
				path:        "root.inactive",
				expectError: true,
			},
			{
				name:        "fails with invalid namespace path",
				setupSQL:    "",
				path:        "invalid-path-%",
				expectError: true,
			},
			{
				name:        "handles transactional error",
				setupSQL:    "ALTER TABLE namespaces RENAME TO namespaces_temp;", // Breaks table availability
				path:        "root.transactionerror",
				expectError: true,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				// Setup
				_, service, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
				now := time.Date(2023, 10, 15, 12, 0, 0, 0, time.UTC)
				ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

				if test.setupSQL != "" {
					_, err := rawDb.Exec(test.setupSQL)
					require.NoError(t, err)
				}

				// Execute
				err := service.EnsureNamespaceByPath(ctx, test.path)

				// Assertions
				if test.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					// Verify namespace exists
					ns, err := service.GetNamespace(ctx, test.path)
					require.NoError(t, err)
					require.Equal(t, test.path, ns.Path)
					require.Equal(t, NamespaceStateActive, ns.State)
				}
			})
		}
	})
	t.Run("UpdateNamespaceLabels", func(t *testing.T) {
		t.Run("set labels on namespace without labels", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.updlabels1",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceLabels(ctx, "root.updlabels1", map[string]string{
				"env":  "prod",
				"team": "backend",
			})
			require.NoError(t, err)
			require.Equal(t, "prod", updated.Labels["env"])
			require.Equal(t, "backend", updated.Labels["team"])

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.updlabels1")
			require.NoError(t, err)
			require.Equal(t, "prod", retrieved.Labels["env"])
			require.Equal(t, "backend", retrieved.Labels["team"])
		})

		t.Run("replaces all existing labels", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.updlabels2",
				State:  NamespaceStateActive,
				Labels: Labels{"old": "value", "also-old": "value2"},
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceLabels(ctx, "root.updlabels2", map[string]string{
				"new": "label",
			})
			require.NoError(t, err)
			require.Equal(t, "label", updated.Labels["new"])
			_, existsOld := updated.Labels["old"]
			require.False(t, existsOld)
			_, existsAlsoOld := updated.Labels["also-old"]
			require.False(t, existsAlsoOld)

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.updlabels2")
			require.NoError(t, err)
			require.Len(t, retrieved.Labels, 1)
			require.Equal(t, "label", retrieved.Labels["new"])
		})

		t.Run("clear labels with empty map", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.updlabels3",
				State:  NamespaceStateActive,
				Labels: Labels{"old": "value"},
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceLabels(ctx, "root.updlabels3", map[string]string{})
			require.NoError(t, err)
			require.Empty(t, updated.Labels)

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.updlabels3")
			require.NoError(t, err)
			require.Empty(t, retrieved.Labels)
		})

		t.Run("nil labels clears labels", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.updlabels4",
				State:  NamespaceStateActive,
				Labels: Labels{"old": "value"},
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceLabels(ctx, "root.updlabels4", nil)
			require.NoError(t, err)
			require.Nil(t, updated.Labels)
		})

		t.Run("namespace not found", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.UpdateNamespaceLabels(ctx, "root.nonexistent", map[string]string{"key": "value"})
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("empty path returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.UpdateNamespaceLabels(ctx, "", map[string]string{"key": "value"})
			require.Error(t, err)
		})

		t.Run("invalid label key returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.updlabels5",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			_, err = db.UpdateNamespaceLabels(ctx, "root.updlabels5", map[string]string{
				"": "empty key",
			})
			require.Error(t, err)
		})

		t.Run("updates timestamp", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			clk := clock.NewFakeClock(now)
			ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.updlabels6",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			original, err := db.GetNamespace(ctx, "root.updlabels6")
			require.NoError(t, err)
			originalUpdatedAt := original.UpdatedAt

			clk.Step(time.Hour)

			updated, err := db.UpdateNamespaceLabels(ctx, "root.updlabels6", map[string]string{"new": "label"})
			require.NoError(t, err)
			require.True(t, updated.UpdatedAt.After(originalUpdatedAt))
		})
	})

	t.Run("PutNamespaceLabels", func(t *testing.T) {
		t.Run("add labels to namespace without labels", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.putlabels1",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceLabels(ctx, "root.putlabels1", map[string]string{
				"env":  "prod",
				"team": "backend",
			})
			require.NoError(t, err)
			require.Equal(t, "prod", updated.Labels["env"])
			require.Equal(t, "backend", updated.Labels["team"])

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.putlabels1")
			require.NoError(t, err)
			require.Equal(t, "prod", retrieved.Labels["env"])
			require.Equal(t, "backend", retrieved.Labels["team"])
		})

		t.Run("add labels to namespace with existing labels", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.putlabels2",
				State:  NamespaceStateActive,
				Labels: Labels{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceLabels(ctx, "root.putlabels2", map[string]string{
				"new": "label",
			})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Labels["existing"])
			require.Equal(t, "label", updated.Labels["new"])
		})

		t.Run("update existing label", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.putlabels3",
				State:  NamespaceStateActive,
				Labels: Labels{"env": "dev"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceLabels(ctx, "root.putlabels3", map[string]string{
				"env": "prod",
			})
			require.NoError(t, err)
			require.Equal(t, "prod", updated.Labels["env"])
		})

		t.Run("multiple labels at once", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.putlabels4",
				State:  NamespaceStateActive,
				Labels: Labels{"keep": "this"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceLabels(ctx, "root.putlabels4", map[string]string{
				"label1": "value1",
				"label2": "value2",
				"label3": "value3",
			})
			require.NoError(t, err)
			require.Equal(t, "this", updated.Labels["keep"])
			require.Equal(t, "value1", updated.Labels["label1"])
			require.Equal(t, "value2", updated.Labels["label2"])
			require.Equal(t, "value3", updated.Labels["label3"])
		})

		t.Run("empty labels map returns current namespace", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.putlabels5",
				State:  NamespaceStateActive,
				Labels: Labels{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceLabels(ctx, "root.putlabels5", map[string]string{})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Labels["existing"])
		})

		t.Run("namespace not found", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.PutNamespaceLabels(ctx, "root.nonexistent", map[string]string{"key": "value"})
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("empty path returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.PutNamespaceLabels(ctx, "", map[string]string{"key": "value"})
			require.Error(t, err)
		})

		t.Run("invalid label key returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.putlabels6",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			_, err = db.PutNamespaceLabels(ctx, "root.putlabels6", map[string]string{
				"": "empty key",
			})
			require.Error(t, err)
		})

		t.Run("updates timestamp", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			clk := clock.NewFakeClock(now)
			ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.putlabels7",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			original, err := db.GetNamespace(ctx, "root.putlabels7")
			require.NoError(t, err)
			originalUpdatedAt := original.UpdatedAt

			// Advance the clock
			clk.Step(time.Hour)

			updated, err := db.PutNamespaceLabels(ctx, "root.putlabels7", map[string]string{"new": "label"})
			require.NoError(t, err)
			require.True(t, updated.UpdatedAt.After(originalUpdatedAt))
		})
	})

	t.Run("DeleteNamespaceLabels", func(t *testing.T) {
		t.Run("delete single label", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.dellabels1",
				State:  NamespaceStateActive,
				Labels: Labels{"env": "prod", "team": "backend"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceLabels(ctx, "root.dellabels1", []string{"env"})
			require.NoError(t, err)
			_, exists := updated.Labels["env"]
			require.False(t, exists)
			require.Equal(t, "backend", updated.Labels["team"])

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.dellabels1")
			require.NoError(t, err)
			_, exists = retrieved.Labels["env"]
			require.False(t, exists)
			require.Equal(t, "backend", retrieved.Labels["team"])
		})

		t.Run("delete multiple labels", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.dellabels2",
				State:  NamespaceStateActive,
				Labels: Labels{"a": "1", "b": "2", "c": "3", "d": "4"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceLabels(ctx, "root.dellabels2", []string{"a", "c"})
			require.NoError(t, err)
			require.Len(t, updated.Labels, 2)
			_, existsA := updated.Labels["a"]
			_, existsC := updated.Labels["c"]
			require.False(t, existsA)
			require.False(t, existsC)
			require.Equal(t, "2", updated.Labels["b"])
			require.Equal(t, "4", updated.Labels["d"])
		})

		t.Run("delete non-existent label is no-op", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.dellabels3",
				State:  NamespaceStateActive,
				Labels: Labels{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceLabels(ctx, "root.dellabels3", []string{"nonexistent"})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Labels["existing"])
		})

		t.Run("delete from namespace without labels", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.dellabels4",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceLabels(ctx, "root.dellabels4", []string{"any"})
			require.NoError(t, err)
			require.Empty(t, updated.Labels)
		})

		t.Run("empty keys slice returns current namespace", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.dellabels5",
				State:  NamespaceStateActive,
				Labels: Labels{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceLabels(ctx, "root.dellabels5", []string{})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Labels["existing"])
		})

		t.Run("namespace not found", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.DeleteNamespaceLabels(ctx, "root.nonexistent", []string{"key"})
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("empty path returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.DeleteNamespaceLabels(ctx, "", []string{"key"})
			require.Error(t, err)
		})

		t.Run("updates timestamp", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			clk := clock.NewFakeClock(now)
			ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:   "root.dellabels6",
				State:  NamespaceStateActive,
				Labels: Labels{"to-delete": "value"},
			})
			require.NoError(t, err)

			original, err := db.GetNamespace(ctx, "root.dellabels6")
			require.NoError(t, err)
			originalUpdatedAt := original.UpdatedAt

			// Advance the clock
			clk.Step(time.Hour)

			updated, err := db.DeleteNamespaceLabels(ctx, "root.dellabels6", []string{"to-delete"})
			require.NoError(t, err)
			require.True(t, updated.UpdatedAt.After(originalUpdatedAt))
		})
	})

	t.Run("Labels", func(t *testing.T) {
		_, db, _ := MustApplyBlankTestDbConfigRaw(t, nil)
		now := time.Date(2023, 10, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Seed data for Namespaces
		namespaces := []*Namespace{
			{
				Path:   "root.n1",
				State:  NamespaceStateActive,
				Labels: Labels{"type": "system"},
			},
			{
				Path:   "root.n2",
				State:  NamespaceStateActive,
				Labels: Labels{"type": "user", "active": "true"},
			},
			{
				Path:   "root.n3",
				State:  NamespaceStateActive,
				Labels: Labels{"type": "user", "active": "false"},
			},
		}

		for _, ns := range namespaces {
			require.NoError(t, db.CreateNamespace(ctx, ns))
		}

		t.Run("Namespace labels", func(t *testing.T) {
			pr := db.ListNamespacesBuilder().
				ForLabelSelector("type=user,active=true").
				FetchPage(ctx)
			require.NoError(t, pr.Error)
			require.Len(t, pr.Results, 1)
			require.Equal(t, "root.n2", pr.Results[0].Path)
		})
	})
}
