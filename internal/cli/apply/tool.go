package apply

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
)

// resolveToolConnection canonicalizes an authored Tool connection reference on
// a detached document before reconciliation. The server projects references as
// IDs, so comparing named references directly would publish needless revisions
// or retain an old ID when a manifest retargets the Tool. A prepared batch keeps
// the resolved ID; a later Prepare resolves the manifest's name again.
func (c *Client) resolveToolConnection(ctx context.Context, doc Document, current *LiveResource) (Document, error) {
	if doc.Kind != toolschema.ToolKind {
		return doc, nil
	}
	if err := validateToolApplyTarget(current); err != nil {
		return Document{}, err
	}
	tool, ok := doc.Resource.(*toolschema.Tool)
	if !ok || tool == nil {
		return Document{}, fmt.Errorf("expected a Tool resource")
	}
	ref := tool.Spec.ConnectionRef
	if ref == (meta.ObjectReference{}) {
		// Partial updates can preserve the existing connection without a lookup.
		return doc, nil
	}
	namespace := doc.Metadata.Namespace
	if current != nil {
		namespace = current.Metadata.Namespace
	}
	if err := toolschema.ValidateConnectionReference(ref, namespace, nil); err != nil {
		return Document{}, fmt.Errorf("invalid Tool connection reference")
	}
	connection, err := c.Resolve(ctx, Document{
		Kind:     ref.Kind,
		Metadata: meta.ObjectMeta{ID: ref.ID, Name: ref.Name, Namespace: ref.Namespace},
	})
	if err != nil {
		return Document{}, fmt.Errorf("Tool connection reference: %w", err)
	}
	if connection == nil || connection.Metadata.Namespace != namespace {
		return Document{}, fmt.Errorf("Tool connection must exist in the Tool namespace")
	}

	object, err := plainObject(doc.Object)
	if err != nil {
		return Document{}, err
	}
	spec := asMap(object["spec"])
	spec["connectionRef"] = map[string]any{
		"apiVersion": string(meta.APIVersionV1Alpha1), "kind": "Connection", "id": connection.Metadata.ID,
	}
	object["spec"] = spec
	return documentWithObject(doc, object)
}

// validateToolApplyTarget rejects controller-owned Tools even for an otherwise
// unchanged plan. Applying or adopting history must not claim ownership of a
// ToolSet's generated inventory. Only a validated owner ID is included in errors.
func validateToolApplyTarget(current *LiveResource) error {
	if current == nil {
		return nil
	}
	tool, ok := current.Resource.(*toolschema.Tool)
	if !ok || tool == nil || tool.Status == nil || tool.Status.ManagedBy == nil {
		return nil
	}
	owner, err := apid.Parse(tool.Status.ManagedBy.ToolSetRef.ID)
	if err == nil && owner.Prefix() == apid.PrefixToolSet {
		return fmt.Errorf("Tool is managed by ToolSet %s; update the owning ToolSet instead", owner)
	}
	return fmt.Errorf("Tool is managed by a ToolSet; update the owning ToolSet instead")
}
