package rate_limit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func validSpec() RateLimitSpec {
	return RateLimitSpec{
		Selector: Selector{},
		Bucket:   Bucket{},
		Algorithm: Algorithm{
			TokenBucket: &TokenBucket{Capacity: 10, RefillRate: 1},
		},
	}
}

func TestRateLimitResourceValidation(t *testing.T) {
	resource := &RateLimit{
		TypeMeta: meta.NewTypeMeta(RateLimitKind),
		Metadata: meta.ObjectMeta{Namespace: "root.acme"},
		Spec:     validSpec(),
	}
	require.NoError(t, resource.ValidateFor(meta.ValidationModeCreate, nil))

	resource.Metadata.ID = apid.New(apid.PrefixRateLimit).String()
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeCreate, nil), "server-owned on create")

	resource.Metadata.Name = "public-api"
	now := time.Now()
	resource.Metadata.CreatedAt = &now
	resource.Metadata.UpdatedAt = &now
	resource.Status = &RateLimitStatus{EffectiveMode: ModeEnforce}
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	resource.Status.EffectiveMode = ModeObserve
	require.ErrorContains(t, resource.ValidateFor(meta.ValidationModeResponse, nil), "must match")
}

func TestRateLimitScopeValidation(t *testing.T) {
	connectorID := apid.New(apid.PrefixConnector)
	connectionID := apid.New(apid.PrefixConnection)
	connectorRef := &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       "Connector",
		ID:         connectorID.String(),
		Generation: 2,
	}
	connectionRef := &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       ConnectionKind,
		ID:         connectionID.String(),
	}

	require.NoError(t, ValidateScope(&RateLimitScope{ConnectorRef: connectorRef}, nil))
	require.NoError(t, ValidateScope(&RateLimitScope{ConnectionRef: connectionRef}, nil))
	require.ErrorContains(t, ValidateScope(&RateLimitScope{}, nil), "exactly one")
	require.ErrorContains(t, ValidateScope(&RateLimitScope{ConnectorRef: connectorRef, ConnectionRef: connectionRef}, nil), "exactly one")

	connectionRef.Generation = 1
	require.ErrorContains(t, ValidateScope(&RateLimitScope{ConnectionRef: connectionRef}, nil), "does not apply")
}

func TestRateLimitPatchPresenceAndApply(t *testing.T) {
	current := &RateLimit{
		TypeMeta: meta.NewTypeMeta(RateLimitKind),
		Metadata: meta.ObjectMeta{
			ID:        apid.New(apid.PrefixRateLimit).String(),
			Name:      "limit",
			Namespace: "root",
		},
		Spec: validSpec(),
		Status: &RateLimitStatus{
			EffectiveMode: ModeEnforce,
		},
	}
	current.Spec.Scope = &RateLimitScope{ConnectionRef: &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       ConnectionKind,
		ID:         apid.New(apid.PrefixConnection).String(),
	}}

	var patch RateLimitPatch
	require.NoError(t, json.Unmarshal([]byte(`{
      "apiVersion":"authproxy.net/v1alpha1",
      "kind":"RateLimit",
      "metadata":{"labels":{}},
      "spec":{"scope":null,"mode":"observe"}
    }`), &patch))
	require.True(t, patch.Spec.HasScope())
	updated, err := patch.ApplyTo(current, nil)
	require.NoError(t, err)
	require.Nil(t, updated.Spec.Scope)
	require.Equal(t, ModeObserve, updated.Spec.Mode)
	require.Empty(t, updated.Metadata.Labels)

	encoded, err := json.Marshal(patch)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"scope":null`)

	var yamlPatch RateLimitPatch
	require.NoError(t, yaml.Unmarshal([]byte(`
apiVersion: authproxy.net/v1alpha1
kind: RateLimit
metadata: {}
spec:
  algorithm: null
`), &yamlPatch))
	require.ErrorContains(t, yamlPatch.ValidateFor(meta.ValidationModeUpdate, nil), "must not be null")
}
