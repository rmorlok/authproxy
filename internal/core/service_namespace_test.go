package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	dbMock "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/stretchr/testify/assert"
	clock "k8s.io/utils/clock/testing"
)

func TestEnsureNamespaceAncestorPath(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockDB := dbMock.NewMockDB(ctrl)
	logger := aplog.NewNoopLogger()
	svc := &service{
		db:     mockDB,
		logger: logger,
	}

	now := time.Now()
	ctx := apctx.WithClock(context.Background(), clock.NewFakeClock(now))

	tests := []struct {
		name          string
		targetNS      string
		setupMocks    func()
		expectedError error
		expectedPath  string
	}{
		{
			name:     "Valid namespace path with no existing namespaces",
			targetNS: "root.child.grandchild",
			setupMocks: func() {
				// Simulate the first query not finding the namespace
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root").
					Return(nil, database.ErrNotFound)
				mockDB.
					EXPECT().
					CreateNamespace(ctx, dbMock.NamespaceMatcher{ExpectedPath: "root"}).
					Return(nil)

				// Same for child
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root.child").
					Return(nil, database.ErrNotFound)
				mockDB.
					EXPECT().
					CreateNamespace(ctx, dbMock.NamespaceMatcher{ExpectedPath: "root.child"}).
					Return(nil)

				// Same for grandchild
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root.child.grandchild").
					Return(nil, database.ErrNotFound)
				mockDB.
					EXPECT().
					CreateNamespace(ctx, dbMock.NamespaceMatcher{ExpectedPath: "root.child.grandchild"}).
					Return(nil)
			},
			expectedError: nil,
			expectedPath:  "root.child.grandchild",
		},
		{
			name:     "Valid namespace path with partially existing namespaces",
			targetNS: "root.child.grandchild",
			setupMocks: func() {
				// Find the root
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root").
					Return(&database.Namespace{Path: "root"}, nil)

				// Same for child
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root.child").
					Return(&database.Namespace{Path: "root.child"}, nil)

				// Grandchild not found
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root.child.grandchild").
					Return(nil, database.ErrNotFound)
				mockDB.
					EXPECT().
					CreateNamespace(ctx, dbMock.NamespaceMatcher{ExpectedPath: "root.child.grandchild"}).
					Return(nil)
			},
			expectedError: nil,
			expectedPath:  "root.child.grandchild",
		},
		{
			name:     "Invalid namespace path",
			targetNS: "!!invalid!!",
			setupMocks: func() {
				// No mocks needed as validation failure comes before interacting with mocks
			},
			expectedError: errors.New("path must be a child of root"),
		},
		{
			name:     "Database error on GetNamespace",
			targetNS: "root.child",
			setupMocks: func() {
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root").
					Return(nil, errors.New("database error"))
			},
			expectedError: errors.New("database error"),
		},
		{
			name:     "Database error on CreateNamespace",
			targetNS: "root.child",
			setupMocks: func() {
				mockDB.
					EXPECT().
					GetNamespace(ctx, "root").
					Return(nil, database.ErrNotFound)
				mockDB.
					EXPECT().
					CreateNamespace(ctx, dbMock.NamespaceMatcher{ExpectedPath: "root"}).
					Return(errors.New("create namespace failure"))
			},
			expectedError: errors.New("create namespace failure"),
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()
			defer ctrl.Finish()

			ns, err := svc.EnsureNamespaceAncestorPath(ctx, tc.targetNS)

			if tc.expectedError != nil {
				assert.EqualError(t, err, tc.expectedError.Error())
				assert.Nil(t, ns)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ns)
				assert.Equal(t, tc.expectedPath, ns.GetPath())
			}
		})
	}
}
