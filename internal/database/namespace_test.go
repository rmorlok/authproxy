package database

import (
	"reflect"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestNamespaces(t *testing.T) {
	t.Run("path", func(t *testing.T) {
		t.Run("validation", func(t *testing.T) {
			tests := []struct {
				name      string
				path      string
				expectErr bool
			}{
				{
					name:      "ValidRootPath",
					path:      "root",
					expectErr: false,
				},
				{
					name:      "ValidChildPath",
					path:      "root/child",
					expectErr: false,
				},
				{
					name:      "ValidNestedChildPath",
					path:      "root/child/grandchild",
					expectErr: false,
				},
				{
					name:      "EmptyPath",
					path:      "",
					expectErr: true,
				},
				{
					name:      "PathNotStartingWithRoot",
					path:      "notroot/child",
					expectErr: true,
				},
				{
					name:      "PathWithInvalidCharacter",
					path:      "root/child@123",
					expectErr: true,
				},
				{
					name:      "PathWithUppercaseLetter",
					path:      "root/Child",
					expectErr: false,
				},
				{
					name:      "PathContainingSpace",
					path:      "root/child with space",
					expectErr: true,
				},
				{
					name:      "PathWithTrailingSlash",
					path:      "root/child/",
					expectErr: true,
				},
				{
					name:      "PathWithSpecialCharacters",
					path:      "root/child!@#",
					expectErr: true,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					err := ValidateNamespacePath(tt.path)
					if tt.expectErr {
						if err == nil {
							t.Errorf("expected error but got nil")
						}
					} else {
						if err != nil {
							t.Errorf("did not expect error but got: %v", err)
						}
					}
				})
			}
		})
		t.Run("splitting", func(t *testing.T) {
			tests := []struct {
				name     string
				path     string
				prefixes []string
			}{
				{
					name:     "root",
					path:     "root",
					prefixes: []string{"root"},
				},
				{
					name:     "single child",
					path:     "root/child",
					prefixes: []string{"root", "root/child"},
				},
				{
					name:     "grandchild",
					path:     "root/child/grandchild",
					prefixes: []string{"root", "root/child", "root/child/grandchild"},
				},
				{
					name:     "empty path",
					path:     "",
					prefixes: []string{},
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					prefixes := SplitNamespacePathToPrefixes(tt.path)
					if !reflect.DeepEqual(prefixes, tt.prefixes) {
						t.Errorf("expected prefixes %v, got %v", tt.prefixes, prefixes)
					}
				})
			}
		})
	})
	t.Run("basic", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw("namespaces", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		sql := `
INSERT INTO namespaces 
(path,              state,       created_at,            updated_at,            deleted_at) VALUES 
('root',            'active',    '2023-10-01 00:00:00', '2023-11-01 00:00:00', null),
('root/prod',       'active',    '2023-10-02 00:00:00', '2023-11-02 00:00:00', null),
('root/prod/12345', 'active',    '2023-10-04 00:00:00', '2023-11-03 00:00:00', null),
('root/prod/54321', 'active',    '2023-10-03 00:00:00', '2023-11-04 00:00:00', null),
('root/prod/99999', 'destroyed', '2023-10-03 03:00:00', '2023-11-04 00:00:00', null),
('root/prod/88888', 'destroyed', '2023-10-03 04:00:00', '2023-11-04 04:00:00', '2023-11-04 05:00:00'),
('root/dev',        'active',    '2023-10-02 01:00:00', '2023-11-05 00:00:00', null)
`
		_, err := rawDb.Exec(sql)
		require.NoError(t, err)

		ns, err := db.GetNamespace(ctx, "root")
		require.NoError(t, err)
		require.Equal(t, "root", ns.Path)
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
		require.Equal(t, count, 6)
	})
	t.Run("CreateNamespace", func(t *testing.T) {
		t.Run("creates a new namespace", func(t *testing.T) {
			// Setup
			_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
			now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
			ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

			ns := &Namespace{
				Path:  "root",
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
				Path:  "root",
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
				Path:  "root",
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
				Path:  "root",
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
				Path:  "root",
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
				Path:  "root",
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
			Path:  "root",
			State: NamespaceStateActive,
		}

		err := db.CreateNamespace(ctx, ns)
		require.NoError(t, err)

		retrieved, err := db.GetNamespace(ctx, "root")
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
			Path:  "root",
			State: NamespaceStateActive,
		}

		err := db.CreateNamespace(ctx, ns)
		require.NoError(t, err)

		retrieved, err := db.GetNamespace(ctx, "root")
		require.NoError(t, err)
		require.Equal(t, ns.Path, retrieved.Path)
		require.Equal(t, ns.State, retrieved.State)

		err = db.DeleteNamespace(ctx, "root")
		require.NoError(t, err)

		retrieved, err = db.GetNamespace(ctx, "root")
		require.ErrorIs(t, err, ErrNotFound)
		require.Nil(t, retrieved)
	})
	t.Run("SetNamespaceState", func(t *testing.T) {
		// Setup
		_, db := MustApplyBlankTestDbConfig(t.Name(), nil)
		now := time.Date(2023, time.October, 15, 12, 0, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		ns := &Namespace{
			Path:  "root",
			State: NamespaceStateActive,
		}

		err := db.CreateNamespace(ctx, ns)
		require.NoError(t, err)

		retrieved, err := db.GetNamespace(ctx, "root")
		require.NoError(t, err)
		require.Equal(t, ns.Path, retrieved.Path)
		require.Equal(t, ns.State, retrieved.State)

		err = db.SetNamespaceState(ctx, "root", NamespaceStateDestroying)
		require.NoError(t, err)

		retrieved, err = db.GetNamespace(ctx, "root")
		require.NoError(t, err)
		require.Equal(t, ns.Path, "root")
		require.Equal(t, NamespaceStateDestroying, retrieved.State)
	})
}
