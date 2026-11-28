package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// ListToolSetGenerations reads every generation of the specified ToolSet in
// the server's order. Results must belong to that ToolSet and identify unique
// nonzero generations.
func (c *Client) ListToolSetGenerations(
	ctx context.Context,
	id apid.ID,
	orderBy string,
) ([]*LiveResource, error) {
	path, err := toolSetGenerationsPath(id)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if orderBy != "" {
		query.Set("orderBy", orderBy)
	}
	return c.listResources(ctx, toolsets.ToolSetKind, path, query, func(m meta.ObjectMeta) (string, error) {
		if m.ID != id.String() || m.Generation == 0 {
			return "", fmt.Errorf("unexpected identity in ToolSet generation list")
		}
		return strconv.FormatUint(m.Generation, 10), nil
	})
}

// CreateToolSetGeneration asks the server to clone the newest definition into
// a new draft. It sends one request and requires a draft in the response.
func (c *Client) CreateToolSetGeneration(ctx context.Context, id apid.ID) (*LiveResource, error) {
	path, err := toolSetGenerationsPath(id)
	if err != nil {
		return nil, err
	}
	return c.writeToolSetGeneration(ctx, id, 0, http.MethodPost, path, nil, toolsets.ToolSetReleaseStateDraft)
}

// UpdateToolSetGeneration replaces an addressed draft's complete definition.
// The patch carries no release intent or logical ToolSet fields.
func (c *Client) UpdateToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
	definition *toolsets.ToolSetDefinition,
) (*LiveResource, error) {
	if err := validateToolSetDefinition(definition); err != nil {
		return nil, err
	}
	patch := toolsets.NewToolSetPatch()
	patch.Spec.Definition = definition
	return c.patchToolSetGeneration(ctx, id, generation, patch, toolsets.ToolSetReleaseStateDraft)
}

// PublishToolSetGeneration requests primary state for the addressed generation.
// A successful API response for a historical generation can be a no-op, so the
// client confirms the observed state before reporting publication.
func (c *Client) PublishToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
) (*LiveResource, error) {
	patch := toolsets.NewToolSetPatch()
	primary := toolsets.ToolSetReleaseStatePrimary
	patch.Spec.Release = &toolsets.ToolSetReleaseSpecPatch{DesiredState: &primary}
	return c.patchToolSetGeneration(ctx, id, generation, patch, primary)
}

// patchToolSetGeneration sends the exact generation patch and verifies the
// resulting state without attempting to create a missing generation.
func (c *Client) patchToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
	patch *toolsets.ToolSetPatch,
	expectedState toolsets.ToolSetReleaseState,
) (*LiveResource, error) {
	path, err := toolSetGenerationsPath(id)
	if err != nil {
		return nil, err
	}
	if generation == 0 {
		return nil, fmt.Errorf("ToolSet generation must be positive")
	}

	body, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("cannot encode ToolSet generation patch")
	}

	return c.writeToolSetGeneration(ctx, id, generation, http.MethodPatch, path+"/"+strconv.FormatUint(generation, 10), body, expectedState)
}

// toolSetGenerationsPath validates the ToolSet ID before building a path.
func toolSetGenerationsPath(id apid.ID) (string, error) {
	if err := toolsets.ValidateID(id.String()); err != nil {
		return "", fmt.Errorf("invalid ToolSet ID")
	}
	descriptor, err := resourceType(toolsets.ToolSetKind)
	if err != nil {
		return "", err
	}
	return pathToOptionalGeneration(descriptor.Collection, id.String(), 0) + "/generations", nil
}

// writeToolSetGeneration submits the mutation once and checks the returned
// generation identity and observed release state. Generation zero is used only
// when creation assigns the number.
func (c *Client) writeToolSetGeneration(
	ctx context.Context,
	id apid.ID,
	generation uint64,
	method, path string,
	body []byte,
	expectedState toolsets.ToolSetReleaseState,
) (*LiveResource, error) {
	data, redacted, err := c.request(ctx, method, path, nil, body)
	if err != nil {
		return nil, err
	}
	live, err := c.decodeLive(toolsets.ToolSetKind, data, redacted)
	if err != nil {
		return nil, err
	}
	if live.Metadata.ID != id.String() || live.Metadata.Generation == 0 ||
		(generation != 0 && live.Metadata.Generation != generation) {
		return nil, fmt.Errorf("unexpected ToolSet generation response identity")
	}

	resource := live.Resource.(*toolsets.ToolSet)
	if resource.Status == nil || resource.Status.Release.State != expectedState {
		return nil, fmt.Errorf("ToolSet generation response did not confirm %s state", expectedState)
	}
	return live, nil
}
