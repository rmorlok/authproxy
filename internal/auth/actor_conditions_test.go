package auth

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestActorValidatorIsAdmin(t *testing.T) {
	t.Parallel()
	notAdmin := database.Actor{}
	valid, reason := ActorValidatorIsAdmin(&notAdmin)
	require.False(t, valid)
	require.NotEmpty(t, reason)

	admin := database.Actor{Admin: true}
	valid, reason = ActorValidatorIsAdmin(&admin)
	require.True(t, valid)
	require.Empty(t, reason)
}

func TestValidateAllActorValidators(t *testing.T) {
	t.Parallel()
	multiple := []ActorValidator{
		ActorValidatorIsAdmin,
		func(a *database.Actor) (bool, string) {
			if a.ExternalId == "bob" {
				return true, ""
			}

			return false, "invalid external id"
		},
	}

	require.True(t, util.First2(validateAllActorValidators(multiple, &database.Actor{Admin: true, ExternalId: "bob"})))
	require.False(t, util.First2(validateAllActorValidators(multiple, &database.Actor{Admin: true})))
	require.False(t, util.First2(validateAllActorValidators(multiple, &database.Actor{ExternalId: "bob"})))
	require.True(t, util.First2(validateAllActorValidators(nil, &database.Actor{})))
	require.False(t, util.First2(validateAllActorValidators(nil, nil)))
}

func TestCombineActorValidators(t *testing.T) {
	t.Parallel()
	adminRestrictions := []ActorValidator{ActorValidatorIsAdmin}
	externalIdRestrictions := []ActorValidator{
		func(a *database.Actor) (bool, string) {
			if a.ExternalId == "bob" {
				return true, ""
			}

			return false, "invalid external id"
		},
	}

	combined := combineActorValidators(adminRestrictions, externalIdRestrictions)
	require.Len(t, combined, 2)
	require.True(t, util.First2(validateAllActorValidators(combined, &database.Actor{Admin: true, ExternalId: "bob"})))
	require.False(t, util.First2(validateAllActorValidators(combined, &database.Actor{Admin: true})))
	require.False(t, util.First2(validateAllActorValidators(combined, &database.Actor{ExternalId: "bob"})))

	require.Len(t, adminRestrictions, 1)
	require.True(t, util.First2(validateAllActorValidators(adminRestrictions, &database.Actor{Admin: true, ExternalId: "bob"})))
	require.True(t, util.First2(validateAllActorValidators(adminRestrictions, &database.Actor{Admin: true})))
	require.False(t, util.First2(validateAllActorValidators(adminRestrictions, &database.Actor{ExternalId: "bob"})))

	require.Len(t, externalIdRestrictions, 1)
	require.True(t, util.First2(validateAllActorValidators(externalIdRestrictions, &database.Actor{Admin: true, ExternalId: "bob"})))
	require.False(t, util.First2(validateAllActorValidators(externalIdRestrictions, &database.Actor{Admin: true})))
	require.True(t, util.First2(validateAllActorValidators(externalIdRestrictions, &database.Actor{ExternalId: "bob"})))

	// Ignores nils
	require.Len(t, combineActorValidators(nil, externalIdRestrictions), 1)
}
