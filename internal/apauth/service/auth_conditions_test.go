package service

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestActorValidatorIsAdmin(t *testing.T) {
	t.Parallel()
	notAdmin := database.Actor{}
	valid, reason := AuthValidatorActorIsAdmin(NewAuthenticatedRequestAuth(&notAdmin))
	require.False(t, valid)
	require.NotEmpty(t, reason)

	admin := database.Actor{Admin: true}
	valid, reason = AuthValidatorActorIsAdmin(NewAuthenticatedRequestAuth(&admin))
	require.True(t, valid)
	require.Empty(t, reason)
}

func TestValidateAllActorValidators(t *testing.T) {
	t.Parallel()
	multiple := []AuthValidator{
		AuthValidatorActorIsAdmin,
		func(ra *RequestAuth) (bool, string) {
			if ra.GetActor().ExternalId == "bob" {
				return true, ""
			}

			return false, "invalid external id"
		},
	}

	require.True(t, util.First2(validateAllAuthValidators(multiple, NewAuthenticatedRequestAuth(&database.Actor{Admin: true, ExternalId: "bob"}))))
	require.False(t, util.First2(validateAllAuthValidators(multiple, NewAuthenticatedRequestAuth(&database.Actor{Admin: true}))))
	require.False(t, util.First2(validateAllAuthValidators(multiple, NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))
	require.True(t, util.First2(validateAllAuthValidators(nil, NewAuthenticatedRequestAuth(&database.Actor{}))))
	require.False(t, util.First2(validateAllAuthValidators(nil, nil)))
}

func TestCombineActorValidators(t *testing.T) {
	t.Parallel()
	adminRestrictions := []AuthValidator{AuthValidatorActorIsAdmin}
	externalIdRestrictions := []AuthValidator{
		func(ra *RequestAuth) (bool, string) {
			if ra.GetActor().ExternalId == "bob" {
				return true, ""
			}

			return false, "invalid external id"
		},
	}

	combined := combineAuthValidators(adminRestrictions, externalIdRestrictions)
	require.Len(t, combined, 2)
	require.True(t, util.First2(validateAllAuthValidators(combined, NewAuthenticatedRequestAuth(&database.Actor{Admin: true, ExternalId: "bob"}))))
	require.False(t, util.First2(validateAllAuthValidators(combined, NewAuthenticatedRequestAuth(&database.Actor{Admin: true}))))
	require.False(t, util.First2(validateAllAuthValidators(combined, NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))

	require.Len(t, adminRestrictions, 1)
	require.True(t, util.First2(validateAllAuthValidators(adminRestrictions, NewAuthenticatedRequestAuth(&database.Actor{Admin: true, ExternalId: "bob"}))))
	require.True(t, util.First2(validateAllAuthValidators(adminRestrictions, NewAuthenticatedRequestAuth(&database.Actor{Admin: true}))))
	require.False(t, util.First2(validateAllAuthValidators(adminRestrictions, NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))

	require.Len(t, externalIdRestrictions, 1)
	require.True(t, util.First2(validateAllAuthValidators(externalIdRestrictions, NewAuthenticatedRequestAuth(&database.Actor{Admin: true, ExternalId: "bob"}))))
	require.False(t, util.First2(validateAllAuthValidators(externalIdRestrictions, NewAuthenticatedRequestAuth(&database.Actor{Admin: true}))))
	require.True(t, util.First2(validateAllAuthValidators(externalIdRestrictions, NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))

	// Ignores nils
	require.Len(t, combineAuthValidators(nil, externalIdRestrictions), 1)
}
