package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
)

// resolveApplyTarget consults the optional registry lifecycle capability.
// Ordinary Resolve retains GET semantics for resource references.
func (c *Client) resolveApplyTarget(ctx context.Context, doc Document) (*LiveResource, error) {
	selected, err := c.Resolve(ctx, doc)
	if err != nil || selected == nil || doc.Metadata.Generation != 0 {
		return selected, err
	}
	descriptor, err := resourceType(doc.Kind)
	if err != nil {
		return nil, err
	}
	if descriptor.Generations == nil {
		return selected, nil
	}
	return c.selectGenerationTarget(ctx, doc, selected, descriptor)
}

// selectGenerationTarget supplies snapshots to the registered policy. The policy
// chooses a source and owns any context carried into patch finalization.
func (c *Client) selectGenerationTarget(ctx context.Context, doc Document, selected *LiveResource, descriptor registry.ResourceType) (*LiveResource, error) {
	editable, newest, err := c.resourceGenerations(ctx, selected, descriptor)
	if err != nil {
		return nil, err
	}
	// Selection uses overwrite only to learn whether the desired generation is
	// already satisfied. Preparation enforces the caller's actual drift policy.
	plan, planErr := Reconcile(Target{Document: doc, Current: selected}, ReconcileOptions{Overwrite: true})
	changed := planErr != nil
	if planErr == nil && plan.Operation == OperationUpdate {
		patch, err := descriptor.DecodePatchJSON(plan.Patch)
		if err != nil {
			return nil, err
		}
		changed, err = descriptor.Generations.ChangesGeneration(patch)
		if err != nil {
			return nil, err
		}
	}
	selection, err := descriptor.Generations.Select(doc.Resource, selected.Resource, changed, editable != nil)
	if err != nil {
		return nil, err
	}
	var chosen *LiveResource
	switch selection.Source {
	case meta.GenerationSelected:
		chosen = selected
	case meta.GenerationEditableSource:
		chosen = editable
	case meta.GenerationNewest:
		chosen = newest
	default:
		return nil, fmt.Errorf("invalid generation source from lifecycle policy")
	}
	if chosen == nil {
		return nil, fmt.Errorf("requested generation source is not available")
	}
	result := *chosen
	result.generationContext = selection.Context
	return &result, nil
}

// Read every page, checking the identity and pagination contract before using
// generation state. A filtered or malformed response must never become a create.
func (c *Client) resourceGenerations(ctx context.Context, selected *LiveResource, descriptor registry.ResourceType) (editable, newest *LiveResource, err error) {
	path := descriptor.Collection + "/" + url.PathEscape(selected.Metadata.ID) + "/generations"
	query := url.Values{}
	cursors := map[string]bool{}
	generations := map[uint64]bool{}
	for {
		data, redacted, err := c.request(ctx, http.MethodGet, path, query, nil)
		if err != nil {
			return nil, nil, err
		}
		var page apiv1alpha1.ResourceList[json.RawMessage]
		if util.DecodeJSONStrict(data, &page) != nil ||
			page.Kind != apiv1alpha1.ListKind(descriptor.GVK.Kind) ||
			page.APIVersion != meta.APIVersionV1Alpha1 ||
			page.Items == nil {
			return nil, nil, fmt.Errorf("invalid resource generation list response")
		}
		for _, item := range page.Items {
			live, err := c.decodeLive(descriptor.GVK.Kind, item, redacted)
			if err != nil {
				return nil, nil, err
			}
			if live.Metadata.ID != selected.Metadata.ID ||
				live.Metadata.Name != selected.Metadata.Name ||
				live.Metadata.Namespace != selected.Metadata.Namespace ||
				live.Metadata.Generation == 0 || generations[live.Metadata.Generation] {
				return nil, nil, fmt.Errorf("inconsistent resource generation list")
			}
			generations[live.Metadata.Generation] = true
			state, err := descriptor.Generations.State(live.Resource)
			if err != nil {
				return nil, nil, err
			}
			switch state {
			case meta.GenerationEditable:
				if editable != nil {
					return nil, nil, fmt.Errorf("multiple editable generations found")
				}
				editable = live
			case meta.GenerationPublished, meta.GenerationHistorical:
			default:
				return nil, nil, fmt.Errorf("invalid generation lifecycle classification")
			}
			if newest == nil || live.Metadata.Generation > newest.Metadata.Generation {
				newest = live
			}
		}
		cursor := page.Metadata.Continue
		if cursor == "" {
			if page.Metadata.RemainingItemCount != nil && *page.Metadata.RemainingItemCount > 0 {
				return nil, nil, fmt.Errorf("incomplete resource generation list")
			}
			if !generations[selected.Metadata.Generation] {
				return nil, nil, fmt.Errorf("selected resource generation missing from list")
			}
			return editable, newest, nil
		}
		if cursors[cursor] {
			return nil, nil, fmt.Errorf("resource generation list repeated a pagination cursor")
		}
		cursors[cursor] = true
		query = url.Values{"cursor": {cursor}}
	}
}

// finalizeGenerationPatch delegates only resource semantics. The resulting patch
// still goes through ordinary redaction, immutable-field, and schema validation.
func finalizeGenerationPatch(descriptor registry.ResourceType, doc Document, current *LiveResource, patch map[string]any) (map[string]any, error) {
	data, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("cannot encode generation patch")
	}
	typed, err := descriptor.DecodePatchJSON(data)
	if err != nil {
		return nil, fmt.Errorf("invalid generation patch")
	}
	finalized, err := descriptor.Generations.Finalize(doc.Resource, current.Resource, typed, current.generationContext, doc.Metadata.Generation != 0)
	if err != nil {
		return nil, err
	}
	return plainObject(finalized)
}
