package database

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/apid"
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
	t.Run("SetNamespaceState returns not found for soft-deleted namespace", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig(t, nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		// Verify the root namespace exists
		retrieved, err := db.GetNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, sconfig.RootNamespace, retrieved.Path)

		// Soft-delete the namespace
		err = db.DeleteNamespace(ctx, sconfig.RootNamespace)
		require.NoError(t, err)

		// Attempting to set state on a soft-deleted namespace should return ErrNotFound
		err = db.SetNamespaceState(ctx, sconfig.RootNamespace, NamespaceStateDestroying)
		require.ErrorIs(t, err, ErrNotFound)
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

	t.Run("UpdateNamespaceAnnotations", func(t *testing.T) {
		t.Run("set annotations on namespace without annotations", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.updannot1",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceAnnotations(ctx, "root.updannot1", map[string]string{
				"env":  "prod",
				"team": "backend",
			})
			require.NoError(t, err)
			require.Equal(t, "prod", updated.Annotations["env"])
			require.Equal(t, "backend", updated.Annotations["team"])

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.updannot1")
			require.NoError(t, err)
			require.Equal(t, "prod", retrieved.Annotations["env"])
			require.Equal(t, "backend", retrieved.Annotations["team"])
		})

		t.Run("replaces all existing annotations", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.updannot2",
				State:       NamespaceStateActive,
				Annotations: Annotations{"old": "value", "also-old": "value2"},
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceAnnotations(ctx, "root.updannot2", map[string]string{
				"new": "annotation",
			})
			require.NoError(t, err)
			require.Equal(t, "annotation", updated.Annotations["new"])
			_, existsOld := updated.Annotations["old"]
			require.False(t, existsOld)
			_, existsAlsoOld := updated.Annotations["also-old"]
			require.False(t, existsAlsoOld)

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.updannot2")
			require.NoError(t, err)
			require.Len(t, retrieved.Annotations, 1)
			require.Equal(t, "annotation", retrieved.Annotations["new"])
		})

		t.Run("clear annotations with empty map", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.updannot3",
				State:       NamespaceStateActive,
				Annotations: Annotations{"old": "value"},
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceAnnotations(ctx, "root.updannot3", map[string]string{})
			require.NoError(t, err)
			require.Empty(t, updated.Annotations)

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.updannot3")
			require.NoError(t, err)
			require.Empty(t, retrieved.Annotations)
		})

		t.Run("nil annotations clears annotations", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.updannot4",
				State:       NamespaceStateActive,
				Annotations: Annotations{"old": "value"},
			})
			require.NoError(t, err)

			updated, err := db.UpdateNamespaceAnnotations(ctx, "root.updannot4", nil)
			require.NoError(t, err)
			require.Nil(t, updated.Annotations)
		})

		t.Run("namespace not found", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.UpdateNamespaceAnnotations(ctx, "root.nonexistent", map[string]string{"key": "value"})
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("empty path returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.UpdateNamespaceAnnotations(ctx, "", map[string]string{"key": "value"})
			require.Error(t, err)
		})

		t.Run("invalid annotation key returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.updannot5",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			_, err = db.UpdateNamespaceAnnotations(ctx, "root.updannot5", map[string]string{
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
				Path:  "root.updannot6",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			original, err := db.GetNamespace(ctx, "root.updannot6")
			require.NoError(t, err)
			originalUpdatedAt := original.UpdatedAt

			clk.Step(time.Hour)

			updated, err := db.UpdateNamespaceAnnotations(ctx, "root.updannot6", map[string]string{"new": "annotation"})
			require.NoError(t, err)
			require.True(t, updated.UpdatedAt.After(originalUpdatedAt))
		})
	})

	t.Run("PutNamespaceAnnotations", func(t *testing.T) {
		t.Run("add annotations to namespace without annotations", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.putannot1",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceAnnotations(ctx, "root.putannot1", map[string]string{
				"env":  "prod",
				"team": "backend",
			})
			require.NoError(t, err)
			require.Equal(t, "prod", updated.Annotations["env"])
			require.Equal(t, "backend", updated.Annotations["team"])

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.putannot1")
			require.NoError(t, err)
			require.Equal(t, "prod", retrieved.Annotations["env"])
			require.Equal(t, "backend", retrieved.Annotations["team"])
		})

		t.Run("add annotations to namespace with existing annotations", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.putannot2",
				State:       NamespaceStateActive,
				Annotations: Annotations{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceAnnotations(ctx, "root.putannot2", map[string]string{
				"new": "annotation",
			})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Annotations["existing"])
			require.Equal(t, "annotation", updated.Annotations["new"])
		})

		t.Run("update existing annotation", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.putannot3",
				State:       NamespaceStateActive,
				Annotations: Annotations{"env": "dev"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceAnnotations(ctx, "root.putannot3", map[string]string{
				"env": "prod",
			})
			require.NoError(t, err)
			require.Equal(t, "prod", updated.Annotations["env"])
		})

		t.Run("multiple annotations at once", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.putannot4",
				State:       NamespaceStateActive,
				Annotations: Annotations{"keep": "this"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceAnnotations(ctx, "root.putannot4", map[string]string{
				"annot1": "value1",
				"annot2": "value2",
				"annot3": "value3",
			})
			require.NoError(t, err)
			require.Equal(t, "this", updated.Annotations["keep"])
			require.Equal(t, "value1", updated.Annotations["annot1"])
			require.Equal(t, "value2", updated.Annotations["annot2"])
			require.Equal(t, "value3", updated.Annotations["annot3"])
		})

		t.Run("empty annotations map returns current namespace", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.putannot5",
				State:       NamespaceStateActive,
				Annotations: Annotations{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.PutNamespaceAnnotations(ctx, "root.putannot5", map[string]string{})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Annotations["existing"])
		})

		t.Run("namespace not found", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.PutNamespaceAnnotations(ctx, "root.nonexistent", map[string]string{"key": "value"})
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("empty path returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.PutNamespaceAnnotations(ctx, "", map[string]string{"key": "value"})
			require.Error(t, err)
		})

		t.Run("invalid annotation key returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.putannot6",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			_, err = db.PutNamespaceAnnotations(ctx, "root.putannot6", map[string]string{
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
				Path:  "root.putannot7",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			original, err := db.GetNamespace(ctx, "root.putannot7")
			require.NoError(t, err)
			originalUpdatedAt := original.UpdatedAt

			// Advance the clock
			clk.Step(time.Hour)

			updated, err := db.PutNamespaceAnnotations(ctx, "root.putannot7", map[string]string{"new": "annotation"})
			require.NoError(t, err)
			require.True(t, updated.UpdatedAt.After(originalUpdatedAt))
		})
	})

	t.Run("DeleteNamespaceAnnotations", func(t *testing.T) {
		t.Run("delete single annotation", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.delannot1",
				State:       NamespaceStateActive,
				Annotations: Annotations{"env": "prod", "team": "backend"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceAnnotations(ctx, "root.delannot1", []string{"env"})
			require.NoError(t, err)
			_, exists := updated.Annotations["env"]
			require.False(t, exists)
			require.Equal(t, "backend", updated.Annotations["team"])

			// Verify in database
			retrieved, err := db.GetNamespace(ctx, "root.delannot1")
			require.NoError(t, err)
			_, exists = retrieved.Annotations["env"]
			require.False(t, exists)
			require.Equal(t, "backend", retrieved.Annotations["team"])
		})

		t.Run("delete multiple annotations", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.delannot2",
				State:       NamespaceStateActive,
				Annotations: Annotations{"a": "1", "b": "2", "c": "3", "d": "4"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceAnnotations(ctx, "root.delannot2", []string{"a", "c"})
			require.NoError(t, err)
			require.Len(t, updated.Annotations, 2)
			_, existsA := updated.Annotations["a"]
			_, existsC := updated.Annotations["c"]
			require.False(t, existsA)
			require.False(t, existsC)
			require.Equal(t, "2", updated.Annotations["b"])
			require.Equal(t, "4", updated.Annotations["d"])
		})

		t.Run("delete non-existent annotation is no-op", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.delannot3",
				State:       NamespaceStateActive,
				Annotations: Annotations{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceAnnotations(ctx, "root.delannot3", []string{"nonexistent"})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Annotations["existing"])
		})

		t.Run("delete from namespace without annotations", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.delannot4",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceAnnotations(ctx, "root.delannot4", []string{"any"})
			require.NoError(t, err)
			require.Empty(t, updated.Annotations)
		})

		t.Run("empty keys slice returns current namespace", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.delannot5",
				State:       NamespaceStateActive,
				Annotations: Annotations{"existing": "value"},
			})
			require.NoError(t, err)

			updated, err := db.DeleteNamespaceAnnotations(ctx, "root.delannot5", []string{})
			require.NoError(t, err)
			require.Equal(t, "value", updated.Annotations["existing"])
		})

		t.Run("namespace not found", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.DeleteNamespaceAnnotations(ctx, "root.nonexistent", []string{"key"})
			require.ErrorIs(t, err, ErrNotFound)
		})

		t.Run("empty path returns error", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := db.DeleteNamespaceAnnotations(ctx, "", []string{"key"})
			require.Error(t, err)
		})

		t.Run("updates timestamp", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			clk := clock.NewFakeClock(now)
			ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:        "root.delannot6",
				State:       NamespaceStateActive,
				Annotations: Annotations{"to-delete": "value"},
			})
			require.NoError(t, err)

			original, err := db.GetNamespace(ctx, "root.delannot6")
			require.NoError(t, err)
			originalUpdatedAt := original.UpdatedAt

			// Advance the clock
			clk.Step(time.Hour)

			updated, err := db.DeleteNamespaceAnnotations(ctx, "root.delannot6", []string{"to-delete"})
			require.NoError(t, err)
			require.True(t, updated.UpdatedAt.After(originalUpdatedAt))
		})
	})

	t.Run("CreateNamespaceWithEncryptionKeyId", func(t *testing.T) {
		t.Run("round-trips encryption key id through create and get", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ekId := apid.New(apid.PrefixEncryptionKey)
			err := db.CreateNamespace(ctx, &Namespace{
				Path:            "root.roundtrip",
				State:           NamespaceStateActive,
				EncryptionKeyId: &ekId,
			})
			require.NoError(t, err)

			retrieved, err := db.GetNamespace(ctx, "root.roundtrip")
			require.NoError(t, err)
			require.NotNil(t, retrieved.EncryptionKeyId)
			require.Equal(t, ekId, *retrieved.EncryptionKeyId)
		})

		t.Run("nil encryption key id round-trips as nil", func(t *testing.T) {
			_, db := MustApplyBlankTestDbConfig(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			err := db.CreateNamespace(ctx, &Namespace{
				Path:  "root.nilkey",
				State: NamespaceStateActive,
			})
			require.NoError(t, err)

			retrieved, err := db.GetNamespace(ctx, "root.nilkey")
			require.NoError(t, err)
			require.Nil(t, retrieved.EncryptionKeyId)
		})
	})

	t.Run("EnumerateNamespaceEncryptionTargets", func(t *testing.T) {
		t.Run("returns all non-deleted namespaces with encryption fields", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := rawDb.Exec(`DELETE FROM namespaces`)
			require.NoError(t, err)

			ekId1 := apid.New(apid.PrefixEncryptionKey)
			ekId2 := apid.New(apid.PrefixEncryptionKey)
			ekvId1 := apid.New(apid.PrefixEncryptionKeyVersion)

			_, err = rawDb.Exec(fmt.Sprintf(`
INSERT INTO namespaces
(path, depth, state, encryption_key_id, target_encryption_key_version_id, created_at, updated_at, deleted_at) VALUES
('root',           0, 'active', NULL,  NULL,  '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('root.ns1',       1, 'active', '%s',  NULL,  '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('root.ns2',       1, 'active', '%s',  '%s',  '2023-10-03 00:00:00', '2023-10-03 00:00:00', null),
('root.ns3',       1, 'active', NULL,  NULL,  '2023-10-04 00:00:00', '2023-10-04 00:00:00', '2023-10-05 00:00:00')
`, ekId1, ekId2, ekvId1))
			require.NoError(t, err)

			var collected []NamespaceEncryptionTarget
			err = db.EnumerateNamespaceEncryptionTargets(ctx,
				func(targets []NamespaceEncryptionTarget, lastPage bool) ([]NamespaceTargetEncryptionKeyVersionUpdate, bool, error) {
					collected = append(collected, targets...)
					return nil, false, nil
				},
			)
			require.NoError(t, err)

			// Should exclude deleted namespace (root.ns3)
			require.Len(t, collected, 3)

			// Results ordered by depth, then path
			require.Equal(t, "root", collected[0].Path)
			require.Equal(t, uint64(0), collected[0].Depth)
			require.Nil(t, collected[0].EncryptionKeyId)
			require.Nil(t, collected[0].TargetEncryptionKeyVersionId)

			require.Equal(t, "root.ns1", collected[1].Path)
			require.Equal(t, uint64(1), collected[1].Depth)
			require.NotNil(t, collected[1].EncryptionKeyId)
			require.Equal(t, ekId1, *collected[1].EncryptionKeyId)
			require.Nil(t, collected[1].TargetEncryptionKeyVersionId)

			require.Equal(t, "root.ns2", collected[2].Path)
			require.Equal(t, uint64(1), collected[2].Depth)
			require.NotNil(t, collected[2].EncryptionKeyId)
			require.Equal(t, ekId2, *collected[2].EncryptionKeyId)
			require.NotNil(t, collected[2].TargetEncryptionKeyVersionId)
			require.Equal(t, ekvId1, *collected[2].TargetEncryptionKeyVersionId)
		})

		t.Run("callback can update target encryption key version", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			clk := clock.NewFakeClock(now)
			ctx := apctx.NewBuilderBackground().WithClock(clk).Build()

			_, err := rawDb.Exec(`DELETE FROM namespaces`)
			require.NoError(t, err)

			ekId := apid.New(apid.PrefixEncryptionKey)
			newEkvId := apid.New(apid.PrefixEncryptionKeyVersion)

			_, err = rawDb.Exec(fmt.Sprintf(`
INSERT INTO namespaces
(path, depth, state, encryption_key_id, created_at, updated_at, deleted_at) VALUES
('root',       0, 'active', NULL, '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('root.ns1',   1, 'active', '%s', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null)
`, ekId))
			require.NoError(t, err)

			err = db.EnumerateNamespaceEncryptionTargets(ctx,
				func(targets []NamespaceEncryptionTarget, lastPage bool) ([]NamespaceTargetEncryptionKeyVersionUpdate, bool, error) {
					var updates []NamespaceTargetEncryptionKeyVersionUpdate
					for _, t := range targets {
						if t.EncryptionKeyId != nil {
							updates = append(updates, NamespaceTargetEncryptionKeyVersionUpdate{
								Path:                         t.Path,
								TargetEncryptionKeyVersionId: newEkvId,
							})
						}
					}
					return updates, false, nil
				},
			)
			require.NoError(t, err)

			// Verify the update was persisted
			var targetEkvId *apid.ID
			var targetUpdatedAt *time.Time
			var updatedAt time.Time
			err = rawDb.QueryRow(
				`SELECT target_encryption_key_version_id, target_encryption_key_version_updated_at, updated_at FROM namespaces WHERE path = 'root.ns1'`,
			).Scan(&targetEkvId, &targetUpdatedAt, &updatedAt)
			require.NoError(t, err)
			require.NotNil(t, targetEkvId)
			require.Equal(t, newEkvId, *targetEkvId)
			require.NotNil(t, targetUpdatedAt)
			require.True(t, now.Equal(*targetUpdatedAt), "targetUpdatedAt should match the clock time")
			require.True(t, now.Equal(updatedAt), "updatedAt should match the clock time")

			// Verify namespace without encryption key was NOT updated
			var rootTargetEkvId *apid.ID
			err = rawDb.QueryRow(
				`SELECT target_encryption_key_version_id FROM namespaces WHERE path = 'root'`,
			).Scan(&rootTargetEkvId)
			require.NoError(t, err)
			require.Nil(t, rootTargetEkvId)
		})

		t.Run("callback can stop early", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := rawDb.Exec(`DELETE FROM namespaces`)
			require.NoError(t, err)

			_, err = rawDb.Exec(`
INSERT INTO namespaces
(path, depth, state, created_at, updated_at, deleted_at) VALUES
('root',       0, 'active', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('root.ns1',   1, 'active', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('root.ns2',   1, 'active', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null)
`)
			require.NoError(t, err)

			callCount := 0
			err = db.EnumerateNamespaceEncryptionTargets(ctx,
				func(targets []NamespaceEncryptionTarget, lastPage bool) ([]NamespaceTargetEncryptionKeyVersionUpdate, bool, error) {
					callCount++
					return nil, true, nil // stop immediately
				},
			)
			require.NoError(t, err)
			require.Equal(t, 1, callCount)
		})

		t.Run("callback error is propagated", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := rawDb.Exec(`DELETE FROM namespaces`)
			require.NoError(t, err)

			_, err = rawDb.Exec(`
INSERT INTO namespaces
(path, depth, state, created_at, updated_at, deleted_at) VALUES
('root', 0, 'active', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null)
`)
			require.NoError(t, err)

			expectedErr := fmt.Errorf("test error")
			err = db.EnumerateNamespaceEncryptionTargets(ctx,
				func(targets []NamespaceEncryptionTarget, lastPage bool) ([]NamespaceTargetEncryptionKeyVersionUpdate, bool, error) {
					return nil, false, expectedErr
				},
			)
			require.ErrorIs(t, err, expectedErr)
		})

		t.Run("empty database returns no results", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := rawDb.Exec(`DELETE FROM namespaces`)
			require.NoError(t, err)

			callCount := 0
			err = db.EnumerateNamespaceEncryptionTargets(ctx,
				func(targets []NamespaceEncryptionTarget, lastPage bool) ([]NamespaceTargetEncryptionKeyVersionUpdate, bool, error) {
					callCount++
					require.Empty(t, targets)
					require.True(t, lastPage)
					return nil, false, nil
				},
			)
			require.NoError(t, err)
			require.Equal(t, 1, callCount)
		})

		t.Run("lastPage flag is correct", func(t *testing.T) {
			_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			_, err := rawDb.Exec(`DELETE FROM namespaces`)
			require.NoError(t, err)

			// Insert exactly 3 namespaces; since page size is 100, all should fit in one page
			_, err = rawDb.Exec(`
INSERT INTO namespaces
(path, depth, state, created_at, updated_at, deleted_at) VALUES
('root',       0, 'active', '2023-10-01 00:00:00', '2023-10-01 00:00:00', null),
('root.ns1',   1, 'active', '2023-10-02 00:00:00', '2023-10-02 00:00:00', null),
('root.ns2',   1, 'active', '2023-10-03 00:00:00', '2023-10-03 00:00:00', null)
`)
			require.NoError(t, err)

			var lastPageValues []bool
			err = db.EnumerateNamespaceEncryptionTargets(ctx,
				func(targets []NamespaceEncryptionTarget, lastPage bool) ([]NamespaceTargetEncryptionKeyVersionUpdate, bool, error) {
					lastPageValues = append(lastPageValues, lastPage)
					return nil, false, nil
				},
			)
			require.NoError(t, err)
			require.Len(t, lastPageValues, 1)
			require.True(t, lastPageValues[0])
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

func TestSetNamespaceEncryptionKeyIdAncestorValidation(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2024, time.January, 10, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	// Create namespace hierarchy: root -> root.parent -> root.parent.child -> root.parent.child.grandchild
	for _, path := range []string{"root.parent", "root.parent.child", "root.parent.child.grandchild", "root.sibling"} {
		require.NoError(t, db.CreateNamespace(ctx, &Namespace{
			Path:  path,
			State: NamespaceStateActive,
		}))
	}

	// Create encryption keys in various namespaces
	ekRoot := &EncryptionKey{
		Id:        apid.New(apid.PrefixEncryptionKey),
		Namespace: "root",
		State:     EncryptionKeyStateActive,
	}
	ekParent := &EncryptionKey{
		Id:        apid.New(apid.PrefixEncryptionKey),
		Namespace: "root.parent",
		State:     EncryptionKeyStateActive,
	}
	ekChild := &EncryptionKey{
		Id:        apid.New(apid.PrefixEncryptionKey),
		Namespace: "root.parent.child",
		State:     EncryptionKeyStateActive,
	}
	ekGrandchild := &EncryptionKey{
		Id:        apid.New(apid.PrefixEncryptionKey),
		Namespace: "root.parent.child.grandchild",
		State:     EncryptionKeyStateActive,
	}
	ekSibling := &EncryptionKey{
		Id:        apid.New(apid.PrefixEncryptionKey),
		Namespace: "root.sibling",
		State:     EncryptionKeyStateActive,
	}

	for _, ek := range []*EncryptionKey{ekRoot, ekParent, ekChild, ekGrandchild, ekSibling} {
		require.NoError(t, db.CreateEncryptionKey(ctx, ek))
	}

	t.Run("valid: key in ancestor namespace", func(t *testing.T) {
		// ekRoot is ancestor of root.parent.child
		ns, err := db.SetNamespaceEncryptionKeyId(ctx, "root.parent.child", &ekRoot.Id)
		require.NoError(t, err)
		require.NotNil(t, ns)
		require.NotNil(t, ns.EncryptionKeyId)
		require.Equal(t, ekRoot.Id, *ns.EncryptionKeyId)

		// ekParent is also an ancestor of root.parent.child
		ns, err = db.SetNamespaceEncryptionKeyId(ctx, "root.parent.child", &ekParent.Id)
		require.NoError(t, err)
		require.NotNil(t, ns)
		require.NotNil(t, ns.EncryptionKeyId)
		require.Equal(t, ekParent.Id, *ns.EncryptionKeyId)
	})

	t.Run("rejected: key in same namespace", func(t *testing.T) {
		_, err := db.SetNamespaceEncryptionKeyId(ctx, "root.parent.child", &ekChild.Id)
		require.Error(t, err)
		var httpErr *httperr.Error
		require.True(t, errors.As(err, &httpErr))
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
	})

	t.Run("rejected: key in descendant namespace", func(t *testing.T) {
		_, err := db.SetNamespaceEncryptionKeyId(ctx, "root.parent.child", &ekGrandchild.Id)
		require.Error(t, err)
		var httpErr *httperr.Error
		require.True(t, errors.As(err, &httpErr))
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
	})

	t.Run("rejected: key in sibling namespace", func(t *testing.T) {
		_, err := db.SetNamespaceEncryptionKeyId(ctx, "root.parent.child", &ekSibling.Id)
		require.Error(t, err)
		var httpErr *httperr.Error
		require.True(t, errors.As(err, &httpErr))
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
	})

	t.Run("rejected: key on root namespace", func(t *testing.T) {
		_, err := db.SetNamespaceEncryptionKeyId(ctx, "root", &ekRoot.Id)
		require.Error(t, err)
		var httpErr *httperr.Error
		require.True(t, errors.As(err, &httpErr))
		require.Equal(t, http.StatusBadRequest, httpErr.Status)
		require.Contains(t, httpErr.ResponseMsg, "root namespace")
	})

	t.Run("clear encryption key succeeds", func(t *testing.T) {
		// First set a valid key
		_, err := db.SetNamespaceEncryptionKeyId(ctx, "root.parent.child", &ekRoot.Id)
		require.NoError(t, err)

		// Clear it
		ns, err := db.SetNamespaceEncryptionKeyId(ctx, "root.parent.child", nil)
		require.NoError(t, err)
		require.NotNil(t, ns)
		require.Nil(t, ns.EncryptionKeyId)
	})
}
