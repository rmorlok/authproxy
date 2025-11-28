package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	cfg "github.com/rmorlok/authproxy/internal/config"
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
		_, db, rawDb := MustApplyBlankTestDbConfigRaw("namespaces", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO namespaces 
(path,                   depth, state,       created_at,            updated_at,            deleted_at) VALUES 
('root',                 0,     'active',    '2023-10-01 00:00:00', '2023-11-01 00:00:00', null),
('root/prod',            1,     'active',    '2023-10-02 00:00:00', '2023-11-02 00:00:00', null),
('root/prod/12345',      2,     'active',    '2023-10-04 00:00:00', '2023-11-03 00:00:00', null),
('root/prod/54321',      2,     'active',    '2023-10-03 00:00:00', '2023-11-04 00:00:00', null),
('root/prod/99999',      2,     'destroyed', '2023-10-03 03:00:00', '2023-11-04 01:00:00', null),
('root/prod-like',       1,     'active',    '2023-10-02 00:00:00', '2023-11-02 00:00:00', null),
('root/prod-like/77777', 2,     'destroyed', '2023-10-03 03:00:00', '2023-11-04 02:00:00', null),
('root/prod/88888',      2,     'destroyed', '2023-10-03 04:00:00', '2023-11-04 04:00:00', '2023-11-04 05:00:00'),
('root/dev',             1,     'active',    '2023-10-02 01:00:00', '2023-11-05 00:00:00', null)
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		ns, err := db.GetNamespace(ctx, cfg.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, cfg.RootNamespace, ns.Path)
		require.Equal(t, NamespaceStateActive, ns.State)

		// Namespace doesn't exist
		ns, err = db.GetNamespace(ctx, "does-not-exist")
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, ns)

		pr := db.ListNamespacesBuilder().
			ForPathPrefix("root/prod").
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Path, "root/prod/12345")
		require.Equal(t, pr.Results[1].Path, "root/prod/99999")
		require.Equal(t, pr.Results[2].Path, "root/prod/54321")
		require.Equal(t, pr.Results[3].Path, "root/prod")

		pr = db.ListNamespacesBuilder().
			ForDepth(2).
			OrderBy(NamespaceOrderByUpdatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 4)
		require.Equal(t, pr.Results[0].Path, "root/prod-like/77777")
		require.Equal(t, pr.Results[1].Path, "root/prod/99999")
		require.Equal(t, pr.Results[2].Path, "root/prod/54321")
		require.Equal(t, pr.Results[3].Path, "root/prod/12345")

		pr = db.ListNamespacesBuilder().
			ForChildrenOf("root/prod").
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 3)
		require.Equal(t, pr.Results[0].Path, "root/prod/12345")
		require.Equal(t, pr.Results[1].Path, "root/prod/99999")
		require.Equal(t, pr.Results[2].Path, "root/prod/54321")

		pr = db.ListNamespacesBuilder().
			ForPathPrefix("root/prod").
			ForState(NamespaceStateDestroyed).
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 1)
		require.Equal(t, pr.Results[0].Path, "root/prod/99999")

		pr = db.ListNamespacesBuilder().
			ForPathPrefix("root/prod").
			ForState(NamespaceStateDestroyed).
			IncludeDeleted().
			OrderBy(NamespaceOrderByCreatedAt, pagination.OrderByDesc).
			FetchPage(ctx)
		require.NoError(t, pr.Error)
		require.Len(t, pr.Results, 2)
		require.Equal(t, pr.Results[0].Path, "root/prod/88888")
		require.Equal(t, pr.Results[1].Path, "root/prod/99999")

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
			_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  cfg.RootNamespace,
				State: NamespaceStateActive,
			}

			// Test
			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.NoError(t, err)
			require.NotNil(t, saveNs)
			require.Equal(t, ns.Path, saveNs.Path)
		})

		t.Run("creates a child namespace", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  cfg.RootNamespace,
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			ns = &Namespace{
				Path:  "root/child",
				State: NamespaceStateActive,
			}

			err = db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.NoError(t, err)
			require.NotNil(t, saveNs)
			require.Equal(t, ns.Path, saveNs.Path)
		})

		t.Run("refuses to create a namespace with invalid name", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  cfg.RootNamespace,
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			ns = &Namespace{
				Path:  "root/#invalid#",
				State: NamespaceStateActive,
			}

			err = db.CreateNamespace(ctx, ns)
			require.Error(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, saveNs)
		})

		t.Run("refuses to create an un-rooted namespace", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  cfg.RootNamespace,
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			ns = &Namespace{
				Path:  "child",
				State: NamespaceStateActive,
			}

			err = db.CreateNamespace(ctx, ns)
			require.Error(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, saveNs)
		})

		t.Run("refuses to create a namespace where parent doesn't exist", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  cfg.RootNamespace,
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			ns = &Namespace{
				Path:  "root/does-not-exist/child",
				State: NamespaceStateActive,
			}

			err = db.CreateNamespace(ctx, ns)
			require.Error(t, err)

			// Verify
			saveNs, err := db.GetNamespace(ctx, ns.Path)
			require.ErrorIs(t, err, ErrNotFound)
			require.Nil(t, saveNs)
		})

		t.Run("refuses to create a namespace where parent is deleted", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  cfg.RootNamespace,
				State: NamespaceStateActive,
			}

			err := db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			ns = &Namespace{
				Path:  "root/parent",
				State: NamespaceStateActive,
			}

			err = db.CreateNamespace(ctx, ns)
			require.NoError(t, err)

			err = db.DeleteNamespace(ctx, ns.Path)
			require.NoError(t, err)

			ns = &Namespace{
				Path:  "root/does-not-exist/child",
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
		_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ns := &Namespace{
			Path:  cfg.RootNamespace,
			State: NamespaceStateActive,
		}

		err := db.CreateNamespace(ctx, ns)
		require.NoError(t, err)

		retrieved, err := db.GetNamespace(ctx, cfg.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, ns.Path, retrieved.Path)
		require.Equal(t, ns.State, retrieved.State)
	})
	t.Run("DeleteNamespace", func(t *testing.T) {
		// Setup
		_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ns := &Namespace{
			Path:  cfg.RootNamespace,
			State: NamespaceStateActive,
		}

		err := db.CreateNamespace(ctx, ns)
		require.NoError(t, err)

		retrieved, err := db.GetNamespace(ctx, cfg.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, ns.Path, retrieved.Path)
		require.Equal(t, ns.State, retrieved.State)

		err = db.DeleteNamespace(ctx, cfg.RootNamespace)
		require.NoError(t, err)

		retrieved, err = db.GetNamespace(ctx, cfg.RootNamespace)
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, retrieved)
	})
	t.Run("SetNamespaceState", func(t *testing.T) {
		// Setup
		_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ns := &Namespace{
			Path:  cfg.RootNamespace,
			State: NamespaceStateActive,
		}

		err := db.CreateNamespace(ctx, ns)
		require.NoError(t, err)

		retrieved, err := db.GetNamespace(ctx, cfg.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, ns.Path, retrieved.Path)
		require.Equal(t, ns.State, retrieved.State)

		err = db.SetNamespaceState(ctx, cfg.RootNamespace, NamespaceStateDestroying)
		require.NoError(t, err)

		retrieved, err = db.GetNamespace(ctx, cfg.RootNamespace)
		require.NoError(t, err)
		require.Equal(t, ns.Path, cfg.RootNamespace)
		require.Equal(t, NamespaceStateDestroying, retrieved.State)
	})
	t.Run("normalize", func(t *testing.T) {
		val := Namespace{
			Path:  "root/prod/12345",
			State: NamespaceStateDestroying,
		}

		val.normalize()
		require.Equal(t, "root/prod/12345", val.Path)
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
			Path:  "root/prod/12345",
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
}
