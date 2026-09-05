package jwt

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

func TestNewActorClaim(t *testing.T) {
	id := apid.New(apid.PrefixActor)
	labels := map[string]string{"team": "platform"}
	permissions := []authschema.Permission{{
		Namespace:   "root.platform.**",
		Resources:   []string{"connections"},
		ResourceIds: []string{"cxn_1"},
		Verbs:       []string{"get"},
	}}
	value := &core.Actor{
		Id:          id,
		Name:        "deploy-bot",
		ExternalId:  "deploy-bot@example.com",
		Namespace:   "root.platform",
		Labels:      labels,
		Annotations: map[string]string{"owner": "auth"},
		Permissions: permissions,
	}

	claim := NewActorClaim(value)
	require.Equal(t, meta.APIVersionV1Alpha1, claim.APIVersion)
	require.Equal(t, actorschema.ActorKind, claim.Kind)
	require.Empty(t, claim.Metadata.ID)
	require.Equal(t, value.Name, claim.Metadata.Name)
	require.Equal(t, value.Namespace, claim.Metadata.Namespace)
	require.Equal(t, value.ExternalId, claim.Spec.ExternalId)
	require.Equal(t, value.Permissions, claim.Spec.Permissions)
	require.Nil(t, claim.Spec.SigningKey)
	require.Nil(t, claim.Status)

	labels["team"] = "changed"
	permissions[0].Resources[0] = "actors"
	require.Equal(t, "platform", claim.Metadata.Labels["team"])
	require.Equal(t, "connections", claim.Spec.Permissions[0].Resources[0])

	runtimeActor := claim.ToCoreActor()
	require.True(t, runtimeActor.Id.IsNil())
	require.Equal(t, value.Name, runtimeActor.Name)
	require.Equal(t, claim.Spec.ExternalId, runtimeActor.ExternalId)
	require.Equal(t, claim.Metadata.Namespace, runtimeActor.Namespace)
}

func TestActorClaimJSON(t *testing.T) {
	valid := `{
		"apiVersion":"authproxy.net/v1alpha1",
		"kind":"Actor",
		"metadata":{
			"name":"deploy-bot",
			"namespace":"root.platform",
			"labels":{"team":"platform"},
			"annotations":{"owner":"auth"}
		},
		"spec":{
			"externalId":"deploy-bot@example.com",
			"permissions":[{
				"namespace":"root.platform.**",
				"resources":["connections"],
				"verbs":["get"]
			}]
		}
	}`

	var claim ActorClaim
	require.NoError(t, json.Unmarshal([]byte(valid), &claim))
	require.NoError(t, claim.Validate())
	require.Equal(t, "deploy-bot@example.com", claim.Spec.ExternalId)
	require.Equal(t, "root.platform", claim.Metadata.Namespace)

	roundTrip, err := json.Marshal(&claim)
	require.NoError(t, err)
	require.JSONEq(t, valid, string(roundTrip))
}

func TestActorClaimRejectsLegacyAndForbiddenJSON(t *testing.T) {
	tests := map[string]string{
		"legacy flat actor": `{
			"externalId":"legacy",
			"namespace":"root",
			"permissions":[]
		}`,
		"metadata id": `{
			"apiVersion":"authproxy.net/v1alpha1","kind":"Actor",
			"metadata":{"id":null,"namespace":"root"},
			"spec":{"externalId":"actor"}
		}`,
		"metadata generation": `{
			"apiVersion":"authproxy.net/v1alpha1","kind":"Actor",
			"metadata":{"generation":0,"namespace":"root"},
			"spec":{"externalId":"actor"}
		}`,
		"metadata createdAt": `{
			"apiVersion":"authproxy.net/v1alpha1","kind":"Actor",
			"metadata":{"createdAt":null,"namespace":"root"},
			"spec":{"externalId":"actor"}
		}`,
		"metadata updatedAt": `{
			"apiVersion":"authproxy.net/v1alpha1","kind":"Actor",
			"metadata":{"updatedAt":null,"namespace":"root"},
			"spec":{"externalId":"actor"}
		}`,
		"signing key": `{
			"apiVersion":"authproxy.net/v1alpha1","kind":"Actor",
			"metadata":{"namespace":"root"},
			"spec":{"externalId":"actor","signingKey":null}
		}`,
		"status": `{
			"apiVersion":"authproxy.net/v1alpha1","kind":"Actor",
			"metadata":{"namespace":"root"},
			"spec":{"externalId":"actor"},
			"status":null
		}`,
		"unknown metadata field": `{
			"apiVersion":"authproxy.net/v1alpha1","kind":"Actor",
			"metadata":{"namespace":"root","externalId":"actor"},
			"spec":{"externalId":"actor"}
		}`,
	}

	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			var claim ActorClaim
			require.Error(t, json.Unmarshal([]byte(payload), &claim))
		})
	}
}

func TestActorClaimValidate(t *testing.T) {
	valid := NewActorClaim(&core.Actor{ExternalId: "actor", Namespace: "root"})
	require.NoError(t, valid.Validate())

	now := time.Now()
	tests := map[string]func(*ActorClaim){
		"api version": func(claim *ActorClaim) { claim.APIVersion = "authproxy.net/v2" },
		"kind":        func(claim *ActorClaim) { claim.Kind = "Connection" },
		"namespace":   func(claim *ActorClaim) { claim.Metadata.Namespace = "outside" },
		"id":          func(claim *ActorClaim) { claim.Metadata.ID = "act_123" },
		"generation":  func(claim *ActorClaim) { claim.Metadata.Generation = 1 },
		"created at":  func(claim *ActorClaim) { claim.Metadata.CreatedAt = &now },
		"updated at":  func(claim *ActorClaim) { claim.Metadata.UpdatedAt = &now },
		"external id": func(claim *ActorClaim) { claim.Spec.ExternalId = "" },
		"permission": func(claim *ActorClaim) {
			claim.Spec.Permissions = []authschema.Permission{{Namespace: "root"}}
		},
		"signing key": func(claim *ActorClaim) { claim.Spec.SigningKey = &keyschema.SigningKey{} },
		"status":      func(claim *ActorClaim) { claim.Status = &actorschema.ActorStatus{} },
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			claim := *valid
			mutate(&claim)
			require.Error(t, claim.Validate())
		})
	}
}
