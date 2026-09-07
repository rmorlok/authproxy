package jwt

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
)

// ActorClaim is the restricted Actor resource representation accepted in a
// JWT. It deliberately reuses the canonical Actor fields while applying a
// tighter transport policy: database identity, lifecycle state, and signing
// material are not valid JWT claims.
type ActorClaim actorschema.Actor

type actorClaimJSON ActorClaim

// NewActorClaim creates the restricted resource view of runtime actor data.
// Mutable maps and permission slices are cloned so later builder changes do
// not mutate the caller's actor.
func NewActorClaim(value core.IActorData) *ActorClaim {
	if value == nil {
		return nil
	}

	resource := actorschema.Actor{
		TypeMeta: meta.NewTypeMeta(actorschema.ActorKind),
		Metadata: meta.ObjectMeta{
			Name:        value.GetName(),
			Namespace:   value.GetNamespace(),
			Labels:      cloneStringMap(value.GetLabels()),
			Annotations: cloneStringMap(value.GetAnnotations()),
		},
		Spec: actorschema.ActorSpec{
			ExternalId:  value.GetExternalId(),
			Permissions: actorschema.ClonePermissions(value.GetPermissions()),
		},
	}

	claim := ActorClaim(resource)
	return &claim
}

// ToCoreActor converts the transport claim into the actor representation used
// by authorization and persistence. JWT actor claims never carry a database
// ID.
func (a *ActorClaim) ToCoreActor() *core.Actor {
	if a == nil {
		return nil
	}

	return &core.Actor{
		Id:          apid.Nil,
		Name:        a.Metadata.Name,
		ExternalId:  a.Spec.ExternalId,
		Namespace:   a.Metadata.Namespace,
		Labels:      cloneStringMap(a.Metadata.Labels),
		Annotations: cloneStringMap(a.Metadata.Annotations),
		Permissions: actorschema.ClonePermissions(a.Spec.Permissions),
	}
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return nil
	}

	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

// UnmarshalJSON accepts only the canonical Actor resource fields and rejects
// forbidden fields even when their JSON values are null or zero. This also
// intentionally rejects the pre-v1alpha1 flat actor claim.
func (a *ActorClaim) UnmarshalJSON(data []byte) error {
	if err := rejectForbiddenActorClaimFields(data); err != nil {
		return err
	}

	var decoded actorClaimJSON
	if err := util.DecodeJSONStrict(data, &decoded); err != nil {
		return fmt.Errorf("decode JWT actor resource: %w", err)
	}

	*a = ActorClaim(decoded)
	return nil
}

func rejectForbiddenActorClaimFields(data []byte) error {
	var resource map[string]json.RawMessage
	if err := json.Unmarshal(data, &resource); err != nil {
		return fmt.Errorf("decode JWT actor resource: %w", err)
	}

	if _, present := resource["status"]; present {
		return fmt.Errorf("actor status is forbidden in JWT claims")
	}

	if raw, present := resource["metadata"]; present {
		var metadataFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &metadataFields); err != nil {
			return fmt.Errorf("decode JWT actor metadata: %w", err)
		}
		for _, field := range []string{"id", "generation", "createdAt", "updatedAt"} {
			if _, present := metadataFields[field]; present {
				return fmt.Errorf("actor metadata.%s is forbidden in JWT claims", field)
			}
		}
	}

	if raw, present := resource["spec"]; present {
		var specFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &specFields); err != nil {
			return fmt.Errorf("decode JWT actor spec: %w", err)
		}
		if _, present := specFields["signingKey"]; present {
			return fmt.Errorf("actor spec.signingKey is forbidden in JWT claims")
		}
	}

	return nil
}

// Validate enforces the JWT-specific Actor resource contract.
func (a *ActorClaim) Validate() error {
	if a == nil {
		return fmt.Errorf("actor is required")
	}

	vc := &common.ValidationContext{Path: "$.actor"}
	var result *multierror.Error
	if err := meta.ValidateResource(a.TypeMeta, a.Metadata, meta.ValidationOptions{
		Mode:               meta.ValidationModeResponse,
		Path:               vc,
		ExpectedAPIVersion: meta.APIVersionV1Alpha1,
		ExpectedKind:       actorschema.ActorKind,
		RequireNamespace:   true,
		NamespaceValidator: nschema.ValidatePath,
	}); err != nil {
		result = multierror.Append(result, err)
	}

	if a.Metadata.ID != "" {
		result = multierror.Append(result, vc.NewErrorForField("metadata.id", "is forbidden in JWT claims"))
	}
	if a.Metadata.Generation != 0 {
		result = multierror.Append(result, vc.NewErrorForField("metadata.generation", "is forbidden in JWT claims"))
	}
	if a.Metadata.CreatedAt != nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata.createdAt", "is forbidden in JWT claims"))
	}
	if a.Metadata.UpdatedAt != nil {
		result = multierror.Append(result, vc.NewErrorForField("metadata.updatedAt", "is forbidden in JWT claims"))
	}
	if a.Spec.ExternalId == "" {
		result = multierror.Append(result, vc.NewErrorForField("spec.externalId", "is required"))
	}
	for i := range a.Spec.Permissions {
		if err := a.Spec.Permissions[i].Validate(); err != nil {
			result = multierror.Append(result, vc.PushField("spec").PushField("permissions").PushIndex(i).NewError(err.Error()))
		}
	}
	if a.Spec.SigningKey != nil {
		result = multierror.Append(result, vc.NewErrorForField("spec.signingKey", "is forbidden in JWT claims"))
	}
	if a.Status != nil {
		result = multierror.Append(result, vc.NewErrorForField("status", "is forbidden in JWT claims"))
	}

	return result.ErrorOrNil()
}
