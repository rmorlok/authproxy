package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestApiKeyCredential_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		c := &ApiKeyCredential{
			Id:           apid.New(apid.PrefixApiKeyCredential),
			ConnectionId: apid.New(apid.PrefixConnection),
		}
		require.NoError(t, c.Validate())
	})
	t.Run("missing id", func(t *testing.T) {
		c := &ApiKeyCredential{
			ConnectionId: apid.New(apid.PrefixConnection),
		}
		require.Error(t, c.Validate())
	})
	t.Run("wrong prefix on id", func(t *testing.T) {
		c := &ApiKeyCredential{
			Id:           apid.New(apid.PrefixActor),
			ConnectionId: apid.New(apid.PrefixConnection),
		}
		require.Error(t, c.Validate())
	})
	t.Run("wrong prefix on connection id", func(t *testing.T) {
		c := &ApiKeyCredential{
			Id:           apid.New(apid.PrefixApiKeyCredential),
			ConnectionId: apid.New(apid.PrefixActor),
		}
		require.Error(t, c.Validate())
	})
	t.Run("wrong prefix on created_by actor", func(t *testing.T) {
		wrong := apid.New(apid.PrefixConnection)
		c := &ApiKeyCredential{
			Id:               apid.New(apid.PrefixApiKeyCredential),
			ConnectionId:     apid.New(apid.PrefixConnection),
			CreatedByActorId: &wrong,
		}
		require.Error(t, c.Validate())
	})
}

func TestApiKeyCredentials_RoundTrip(t *testing.T) {
	_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connectionId := apid.New(apid.PrefixConnection)
	actorId := apid.New(apid.PrefixActor)
	placement := &cschema.ApiKeyPlacement{
		Type:       cschema.ApiKeyPlacementHeader,
		HeaderName: "X-API-Key",
		Prefix:     "Token ",
	}
	blob := encfield.EncryptedField{ID: "ekv_test", Data: "encryptedCredentialsBlob"}

	cred, err := db.InsertApiKeyCredential(
		ctx,
		connectionId,
		blob,
		placement,
		&actorId,
	)
	require.NoError(t, err)
	require.NotNil(t, cred)
	require.True(t, cred.Id.HasPrefix(apid.PrefixApiKeyCredential))
	require.Equal(t, connectionId, cred.ConnectionId)
	require.Equal(t, blob, cred.EncryptedCredentials)
	require.Equal(t, placement, cred.PlacementSnapshot)
	require.NotSame(t, placement, cred.PlacementSnapshot, "placement should be cloned on insert")
	require.Equal(t, &actorId, cred.CreatedByActorId)
	require.True(t, now.Equal(cred.CreatedAt))

	got, err := db.GetActiveApiKeyCredential(ctx, connectionId)
	require.NoError(t, err)
	require.Equal(t, cred.Id, got.Id)
	require.Equal(t, blob, got.EncryptedCredentials)
	require.Equal(t, placement, got.PlacementSnapshot)

	require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM api_key_credentials"))
	require.Equal(t, 0, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM api_key_credentials WHERE deleted_at IS NOT NULL"))
}

func TestApiKeyCredentials_GetActive_NoneFound(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	connectionId := apid.New(apid.PrefixConnection)
	got, err := db.GetActiveApiKeyCredential(ctx, connectionId)
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, got)
}

func TestApiKeyCredentials_InsertSoftDeletesPrior(t *testing.T) {
	_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connectionId := apid.New(apid.PrefixConnection)
	placement := &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}

	first, err := db.InsertApiKeyCredential(
		ctx,
		connectionId,
		encfield.EncryptedField{ID: "ekv_test", Data: "firstBlob"},
		placement,
		nil,
	)
	require.NoError(t, err)

	second, err := db.InsertApiKeyCredential(
		ctx,
		connectionId,
		encfield.EncryptedField{ID: "ekv_test", Data: "secondBlob"},
		placement,
		nil,
	)
	require.NoError(t, err)
	require.NotEqual(t, first.Id, second.Id)

	// Exactly one active row remains.
	require.Equal(t, 2, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM api_key_credentials"))
	require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM api_key_credentials WHERE deleted_at IS NULL"))

	got, err := db.GetActiveApiKeyCredential(ctx, connectionId)
	require.NoError(t, err)
	require.Equal(t, second.Id, got.Id)
	require.Equal(t, encfield.EncryptedField{ID: "ekv_test", Data: "secondBlob"}, got.EncryptedCredentials)
}

func TestApiKeyCredentials_InsertIsolatedAcrossConnections(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connectionA := apid.New(apid.PrefixConnection)
	connectionB := apid.New(apid.PrefixConnection)
	placement := &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}

	credA, err := db.InsertApiKeyCredential(
		ctx, connectionA,
		encfield.EncryptedField{ID: "ekv_test", Data: "blobA"},
		placement, nil,
	)
	require.NoError(t, err)

	credB, err := db.InsertApiKeyCredential(
		ctx, connectionB,
		encfield.EncryptedField{ID: "ekv_test", Data: "blobB"},
		placement, nil,
	)
	require.NoError(t, err)

	// Inserting B should not touch A.
	gotA, err := db.GetActiveApiKeyCredential(ctx, connectionA)
	require.NoError(t, err)
	require.Equal(t, credA.Id, gotA.Id)

	gotB, err := db.GetActiveApiKeyCredential(ctx, connectionB)
	require.NoError(t, err)
	require.Equal(t, credB.Id, gotB.Id)
}

func TestApiKeyCredentials_UpdateLastValidated(t *testing.T) {
	_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connectionId := apid.New(apid.PrefixConnection)
	cred, err := db.InsertApiKeyCredential(
		ctx, connectionId,
		encfield.EncryptedField{ID: "ekv_test", Data: "blob"},
		&cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer},
		nil,
	)
	require.NoError(t, err)
	require.Nil(t, cred.LastValidatedAt)

	at := now.Add(5 * time.Minute)
	require.NoError(t, db.UpdateApiKeyCredentialLastValidated(ctx, cred.Id, at))

	got, err := db.GetActiveApiKeyCredential(ctx, connectionId)
	require.NoError(t, err)
	require.NotNil(t, got.LastValidatedAt)
	require.True(t, at.Equal(*got.LastValidatedAt))

	// Updating a non-existent id surfaces ErrNotFound.
	err = db.UpdateApiKeyCredentialLastValidated(ctx, apid.New(apid.PrefixApiKeyCredential), at)
	require.ErrorIs(t, err, ErrNotFound)

	// Ensure we didn't mutate row count.
	require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM api_key_credentials"))
}

func TestApiKeyCredentials_DeleteAllForConnection(t *testing.T) {
	_, db, rawDb := MustApplyBlankTestDbConfigRaw(t, nil)
	now := time.Date(2024, time.March, 15, 10, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connectionId := apid.New(apid.PrefixConnection)
	otherId := apid.New(apid.PrefixConnection)
	placement := &cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}

	// Two consecutive inserts on one connection (the second soft-deletes the first),
	// plus a credential on a sibling connection that must be untouched.
	_, err := db.InsertApiKeyCredential(ctx, connectionId,
		encfield.EncryptedField{ID: "ekv_test", Data: "k1"}, placement, nil)
	require.NoError(t, err)
	_, err = db.InsertApiKeyCredential(ctx, connectionId,
		encfield.EncryptedField{ID: "ekv_test", Data: "k2"}, placement, nil)
	require.NoError(t, err)
	siblingCred, err := db.InsertApiKeyCredential(ctx, otherId,
		encfield.EncryptedField{ID: "ekv_test", Data: "sibling"}, placement, nil)
	require.NoError(t, err)

	require.NoError(t, db.DeleteAllApiKeyCredentialsForConnection(ctx, connectionId))

	got, err := db.GetActiveApiKeyCredential(ctx, connectionId)
	require.ErrorIs(t, err, ErrNotFound)
	require.Nil(t, got)

	siblingGot, err := db.GetActiveApiKeyCredential(ctx, otherId)
	require.NoError(t, err)
	require.Equal(t, siblingCred.Id, siblingGot.Id)

	// Total rows: 2 for connectionId (both soft-deleted) + 1 for sibling (active).
	require.Equal(t, 3, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM api_key_credentials"))
	require.Equal(t, 1, sqlh.MustCount(rawDb, "SELECT COUNT(*) FROM api_key_credentials WHERE deleted_at IS NULL"))
}

func TestApiKeyCredentials_EncryptedFieldRegistration(t *testing.T) {
	regs := GetEncryptedFieldRegistrations()
	var found *EncryptedFieldRegistration
	for i := range regs {
		if regs[i].Table == ApiKeyCredentialsTable {
			found = &regs[i]
			break
		}
	}
	require.NotNil(t, found, "api_key_credentials must register its encrypted column with the re-encryption registry")
	require.ElementsMatch(t, []string{"encrypted_credentials"}, found.EncryptedCols,
		"a single encrypted_credentials column simplifies re-encryption jobs")
	require.Equal(t, ConnectionsTable, found.JoinTable, "namespace should resolve via JOIN to connections")
	require.Equal(t, "connection_id", found.JoinLocalCol)
	require.Equal(t, "id", found.JoinRemoteCol)
	require.Equal(t, "namespace", found.JoinNamespaceCol)
}

func TestApiKeyCredentials_PlacementSnapshotRoundtripsEachVariant(t *testing.T) {
	cases := []struct {
		name string
		p    cschema.ApiKeyPlacement
	}{
		{"bearer", cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBearer}},
		{"header", cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementHeader, HeaderName: "X-API-Key", Prefix: "Token "}},
		{"query", cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementQuery, ParamName: "api_key"}},
		{"basic", cschema.ApiKeyPlacement{Type: cschema.ApiKeyPlacementBasic, UsernameField: "account_id"}},
	}

	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(time.Now())).Build()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connectionId := apid.New(apid.PrefixConnection)
			p := tc.p
			_, err := db.InsertApiKeyCredential(ctx, connectionId,
				encfield.EncryptedField{ID: "ekv_test", Data: "blob"}, &p, nil)
			require.NoError(t, err)

			got, err := db.GetActiveApiKeyCredential(ctx, connectionId)
			require.NoError(t, err)
			require.Equal(t, &tc.p, got.PlacementSnapshot)
		})
	}
}
