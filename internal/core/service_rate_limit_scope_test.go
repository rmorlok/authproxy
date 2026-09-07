package core

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func TestValidateRateLimitScopeTargetNamespace(t *testing.T) {
	for _, namespace := range []string{
		"root.acme",
		"root.acme.payments",
		"root.acme.payments.us",
	} {
		err := validateRateLimitScopeTargetNamespace(
			"connector",
			"root.acme",
			namespace,
		)
		require.NoError(t, err)
	}

	for _, namespace := range []string{
		"root",
		"root.other",
		"root.acmes",
		"root.ac",
	} {
		err := validateRateLimitScopeTargetNamespace(
			"connection",
			"root.acme",
			namespace,
		)
		require.ErrorIs(t, err, ErrInvalidArgument)
		require.ErrorContains(t, err, "outside rate-limit namespace")
	}
}

func TestNormalizeRateLimitConnectorScopeResolvesLogicalConnector(t *testing.T) {
	ctrl := gomock.NewController(t)
	s, db, _, _, _, _ := FullMockService(t, ctrl)
	ctx := context.Background()
	connectorID := apid.New(apid.PrefixConnector)
	ref := meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       cschema.ConnectorKind,
		Namespace:  "root.acme",
		Name:       "billing",
	}
	resource := &rlschema.RateLimit{
		Metadata: meta.ObjectMeta{Namespace: "root.acme"},
		Spec: rlschema.RateLimitSpec{Scope: &rlschema.RateLimitScope{
			ConnectorRef: &ref,
		}},
	}

	db.EXPECT().ResolveConnectorReference(ctx, ref).Return(&database.Connector{
		Id:        connectorID,
		Namespace: "root.acme.payments",
		Name:      "billing",
	}, nil)

	require.NoError(t, s.normalizeRateLimitScope(ctx, resource))
	require.Equal(t, &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       cschema.ConnectorKind,
		ID:         connectorID.String(),
	}, resource.Spec.Scope.ConnectorRef)
}
