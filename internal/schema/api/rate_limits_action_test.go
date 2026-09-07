package api

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/stretchr/testify/require"
)

func TestRateLimitDryRunActionValidatesTargetsAndActor(t *testing.T) {
	base := RateLimitDryRunAction{
		Action: NewRateLimitDryRunResponse(
			meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       namespace.NamespaceKind,
				ID:         "root.acme",
			},
			RateLimitDryRunSpec{
				Request:     ProxyRequestJson{Method: "GET", URL: "https://api.example.com"},
				RequestType: "proxy",
				ActorRef: &meta.ObjectReference{
					APIVersion: meta.APIVersionV1Alpha1,
					Kind:       actor.ActorKind,
					ID:         "act_test",
				},
			},
			RateLimitDryRunStatus{},
		).Action,
	}

	require.NoError(t, base.ValidateResponse(RateLimitDryRunActionKind))

	request := base
	request.Status = nil
	require.NoError(t, request.ValidateRequest(RateLimitDryRunActionKind))

	request.Metadata.Target = meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       connection.ConnectionKind,
		ID:         "cxn_test",
	}
	require.NoError(t, request.ValidateRequest(RateLimitDryRunActionKind))
}

func TestRateLimitDryRunActionRejectsAmbiguousReferences(t *testing.T) {
	action := RateLimitDryRunAction{
		Action: NewRateLimitDryRunResponse(
			meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       namespace.NamespaceKind,
				ID:         "root",
				Name:       "root",
			},
			RateLimitDryRunSpec{
				Request:     ProxyRequestJson{Method: "GET", URL: "https://api.example.com"},
				RequestType: "proxy",
			},
			RateLimitDryRunStatus{},
		).Action,
	}
	action.Status = nil

	require.ErrorContains(t, action.ValidateRequest(RateLimitDryRunActionKind), "support id only")
}
