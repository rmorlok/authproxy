package main

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	dbmock "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/require"
)

func TestSystemApplyActorCreatedOnce(t *testing.T) {
	_, db, _ := database.MustApplyBlankTestDbConfigRaw(t, nil)
	ctx := context.Background()
	require.NoError(t, ensureSystemApplyActor(ctx, db))

	a, err := db.GetActorByExternalId(ctx, "root", "system")
	require.NoError(t, err)
	require.Equal(t, database.Permissions(auth.AllPermissions()), a.Permissions)
	require.Nil(t, a.EncryptedKey)
	require.NoError(t, ensureSystemApplyActor(ctx, db))

	b, err := db.GetActorByExternalId(ctx, "root", "system")
	require.NoError(t, err)
	require.Equal(t, a, b)
}

func TestSystemApplyActorPreservesExistingIdentity(t *testing.T) {
	_, db, _ := database.MustApplyBlankTestDbConfigRaw(t, nil)
	ctx := context.Background()

	require.NoError(t, db.CreateActor(ctx, &database.Actor{
		Id: apid.New(apid.PrefixActor),
		Namespace: "root",
		ExternalId: "system",
		Name: "existing",
		Labels: database.Labels{"team": "ops"},
		EncryptedKey: &encfield.EncryptedField{
			ID: apid.New(apid.PrefixDataEncryptionKey),
			Data: "stored-key",
		},
		Permissions:  auth.PermissionsSingle("root", "connectors", "get"),
	}))

	a, err := db.GetActorByExternalId(ctx, "root", "system")
	require.NoError(t, err)
	require.NoError(t, ensureSystemApplyActor(ctx, db))

	b, err := db.GetActorByExternalId(ctx, "root", "system")
	require.NoError(t, err)
	require.Equal(t, a, b)
}

func TestSystemApplyActorCreationRaceAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		lookupErr, createErr, racedLookupErr error
		wantErr                              error
	}{
		{
			name: "concurrent creation",
			lookupErr: database.ErrNotFound,
			createErr: database.ErrDuplicate,
		},
		{
			name: "deleted or conflicting actor",
			lookupErr: database.ErrNotFound,
			createErr: database.ErrDuplicate,
			racedLookupErr: database.ErrNotFound,
			wantErr: database.ErrNotFound,
		},
		{
			name: "lookup failure",
			lookupErr: context.Canceled,
			wantErr: context.Canceled,
		},
		{
			name: "creation failure",
			lookupErr: database.ErrNotFound,
			createErr: context.Canceled,
			wantErr: context.Canceled,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := dbmock.NewMockDB(gomock.NewController(t))
			lookup := db.EXPECT().
				GetActorByExternalId(gomock.Any(), "root", "system").
				Return(nil, tc.lookupErr)

			if errors.Is(tc.lookupErr, database.ErrNotFound) {
				create := db.EXPECT().
					CreateActor(gomock.Any(), gomock.Any()).After(lookup).
					Return(tc.createErr)

				if errors.Is(tc.createErr, database.ErrDuplicate) {
					db.EXPECT().
						GetActorByExternalId(gomock.Any(), "root", "system").
						After(create).Return(&database.Actor{}, tc.racedLookupErr)
				}
			}

			require.ErrorIs(t, ensureSystemApplyActor(context.Background(), db), tc.wantErr)
		})
	}
}
