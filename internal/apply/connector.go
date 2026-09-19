package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	apiv1alpha1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
)

// resolveApplyTarget chooses the generation that the logical connector update
// endpoint will edit. Resolve itself retains ordinary GET semantics for references.
func (c *Client) resolveApplyTarget(ctx context.Context, doc Document) (*LiveResource, error) {
	selected, err := c.Resolve(ctx, doc)
	if err != nil || selected == nil || doc.Kind != "Connector" || doc.Metadata.Generation != 0 {
		return selected, err
	}
	draft, newest, err := c.connectorGenerations(ctx, selected)
	if err != nil {
		return nil, err
	}

	// A satisfied primary declaration must not publish an unrelated draft. Use
	// overwrite here only to select a target; preparation enforces the real policy.
	plan, selectedErr := Reconcile(Target{Document: doc, Current: selected}, ReconcileOptions{Overwrite: true})
	desired, _ := doc.Resource.(*connectors.Connector)
	if selectedErr == nil && !connectorSpecChanged(plan) &&
		(draft == nil || (desired != nil && desired.Spec.Release.DesiredState == connectors.ConnectorReleaseStatePrimary)) {
		return selected, nil
	}
	selectedConnector := selected.Resource.(*connectors.Connector)
	publishDefinition := desired != nil &&
		desired.Spec.Release.DesiredState == connectors.ConnectorReleaseStatePrimary &&
		selectedConnector.Status != nil &&
		selectedConnector.Status.Release.State == connectors.ConnectorReleaseStatePrimary
	if draft != nil {
		draft.publishDefinition = publishDefinition
		return draft, nil
	}
	// With no draft the server clones the newest generation, which is not
	// necessarily the primary returned by the logical GET.
	if newest != nil {
		newest.requiresDraft = true
		newest.publishDefinition = publishDefinition
		return newest, nil
	}
	return nil, fmt.Errorf("connector has no visible generations")
}

func connectorSpecChanged(plan *Plan) bool {
	if plan.Operation != OperationUpdate {
		return false
	}
	var patch connectors.ConnectorPatch
	if json.Unmarshal(plan.Patch, &patch) != nil {
		return true
	}
	return patch.Spec != nil && (patch.Spec.HasDefinition() || patch.Spec.HasRelease())
}

// Read every page, checking the identity and pagination contract before using
// generation state. A filtered or malformed response must never become a create.
func (c *Client) connectorGenerations(ctx context.Context, selected *LiveResource) (draft, newest *LiveResource, err error) {
	path := "connectors/" + url.PathEscape(selected.Metadata.ID) + "/generations"
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
			page.Kind != apiv1alpha1.ListKind("Connector") ||
			page.APIVersion != meta.APIVersionV1Alpha1 ||
			page.Items == nil {
			return nil, nil, fmt.Errorf("invalid connector generation list response")
		}
		for _, item := range page.Items {
			live, err := c.decodeLive("Connector", item, redacted)
			if err != nil {
				return nil, nil, err
			}
			if live.Metadata.ID != selected.Metadata.ID ||
				live.Metadata.Name != selected.Metadata.Name ||
				live.Metadata.Namespace != selected.Metadata.Namespace ||
				live.Metadata.Generation == 0 || generations[live.Metadata.Generation] {
				return nil, nil, fmt.Errorf("inconsistent connector generation list")
			}
			generations[live.Metadata.Generation] = true
			resource := live.Resource.(*connectors.Connector)
			if resource.Status == nil {
				return nil, nil, fmt.Errorf("connector generation is missing release status")
			}
			switch resource.Status.Release.State {
			case connectors.ConnectorReleaseStateDraft:
				if draft != nil {
					return nil, nil, fmt.Errorf("multiple connector drafts found")
				}
				draft = live
			case connectors.ConnectorReleaseStatePrimary, connectors.ConnectorReleaseStateActive, connectors.ConnectorReleaseStateArchived:
			default:
				return nil, nil, fmt.Errorf("invalid connector release status")
			}
			if newest == nil || live.Metadata.Generation > newest.Metadata.Generation {
				newest = live
			}
		}
		cursor := page.Metadata.Continue
		if cursor == "" {
			if page.Metadata.RemainingItemCount != nil && *page.Metadata.RemainingItemCount > 0 {
				return nil, nil, fmt.Errorf("incomplete connector generation list")
			}
			if !generations[selected.Metadata.Generation] {
				return nil, nil, fmt.Errorf("selected connector generation missing from list")
			}
			return draft, newest, nil
		}
		if cursors[cursor] {
			return nil, nil, fmt.Errorf("connector generation list repeated a pagination cursor")
		}
		cursors[cursor] = true
		query = url.Values{"cursor": {cursor}}
	}
}
