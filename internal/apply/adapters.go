package apply

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/schema/manifest"
	"github.com/rmorlok/authproxy/internal/schema/registry"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

func resourceType(kind meta.Kind) (registry.ResourceType, error) {
	return registry.LookupResource(manifest.GVK{APIVersion: meta.APIVersionV1Alpha1, Kind: kind})
}

func resourceMetadata(resource any) (meta.ObjectMeta, meta.Kind, error) {
	descriptor, err := registry.TypeOf(resource)
	if err != nil {
		return meta.ObjectMeta{}, "", err
	}
	metadata, err := descriptor.Metadata(resource)
	return metadata, descriptor.GVK.Kind, err
}

// decodePatch preserves presence using the canonical patch decoder. Callers
// supply a calculated patch, not the entire live resource (which may be masked).
func decodePatch(
	kind meta.Kind,
	data []byte,
	current any,
) (registry.Value, error) {
	descriptor, err := resourceType(kind)
	if err != nil {
		return nil, err
	}

	patch, err := descriptor.DecodePatchJSON(data)
	if err != nil {
		return nil, fmt.Errorf("invalid %s patch fields, types or semantics", kind)
	}

	if err := apserde.ValidateNoRedactedPlaceholders(patch); err != nil {
		return nil, fmt.Errorf("redacted placeholders cannot be submitted")
	}

	_, currentKind, err := resourceMetadata(current)
	if err != nil || currentKind != kind {
		return nil, fmt.Errorf("patch target kind mismatch")
	}

	if _, err := descriptor.ApplyPatch(current, patch); err != nil {
		return nil, fmt.Errorf("%s patch changes immutable identity/policy or produces an invalid resource", kind)
	}

	return patch, nil
}
