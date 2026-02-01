package service

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

func TestValidateAllActorValidators(t *testing.T) {
	t.Parallel()

	externalIdValidator := func(gctx *gin.Context, ra *core.RequestAuth) (bool, string) {
		if ra.GetActor().ExternalId == "bob" {
			return true, ""
		}
		return false, "invalid external id"
	}

	namespaceValidator := func(gctx *gin.Context, ra *core.RequestAuth) (bool, string) {
		if ra.GetActor().GetNamespace() == "test" {
			return true, ""
		}
		return false, "invalid namespace"
	}

	multiple := []AuthValidator{
		externalIdValidator,
		namespaceValidator,
	}

	require.True(t, util.First2(validateAllAuthValidators(multiple, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob", Namespace: "test"}))))
	require.False(t, util.First2(validateAllAuthValidators(multiple, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))
	require.False(t, util.First2(validateAllAuthValidators(multiple, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{Namespace: "test"}))))
	require.True(t, util.First2(validateAllAuthValidators(nil, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{}))))
	require.False(t, util.First2(validateAllAuthValidators(nil, nil, nil)))
}

func TestCombineActorValidators(t *testing.T) {
	t.Parallel()

	externalIdRestrictions := []AuthValidator{
		func(gctx *gin.Context, ra *core.RequestAuth) (bool, string) {
			if ra.GetActor().ExternalId == "bob" {
				return true, ""
			}
			return false, "invalid external id"
		},
	}

	namespaceRestrictions := []AuthValidator{
		func(gctx *gin.Context, ra *core.RequestAuth) (bool, string) {
			if ra.GetActor().GetNamespace() == "test" {
				return true, ""
			}
			return false, "invalid namespace"
		},
	}

	combined := combineAuthValidators(externalIdRestrictions, namespaceRestrictions)
	require.Len(t, combined, 2)
	require.True(t, util.First2(validateAllAuthValidators(combined, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob", Namespace: "test"}))))
	require.False(t, util.First2(validateAllAuthValidators(combined, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))
	require.False(t, util.First2(validateAllAuthValidators(combined, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{Namespace: "test"}))))

	require.Len(t, externalIdRestrictions, 1)
	require.True(t, util.First2(validateAllAuthValidators(externalIdRestrictions, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob", Namespace: "test"}))))
	require.True(t, util.First2(validateAllAuthValidators(externalIdRestrictions, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))
	require.False(t, util.First2(validateAllAuthValidators(externalIdRestrictions, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{Namespace: "test"}))))

	require.Len(t, namespaceRestrictions, 1)
	require.True(t, util.First2(validateAllAuthValidators(namespaceRestrictions, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob", Namespace: "test"}))))
	require.False(t, util.First2(validateAllAuthValidators(namespaceRestrictions, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{ExternalId: "bob"}))))
	require.True(t, util.First2(validateAllAuthValidators(namespaceRestrictions, &gin.Context{}, core.NewAuthenticatedRequestAuth(&database.Actor{Namespace: "test"}))))

	// Ignores nils
	require.Len(t, combineAuthValidators(nil, externalIdRestrictions), 1)
}
