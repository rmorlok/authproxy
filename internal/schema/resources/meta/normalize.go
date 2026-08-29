package meta

import (
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// NormalizeObjectMeta returns a deep copy with timestamps normalized to UTC.
// It deliberately does not trim or case-fold names, namespaces, labels, or
// annotations because those values are case-sensitive resource data.
func NormalizeObjectMeta(value ObjectMeta) ObjectMeta {
	value.Labels = maps.Clone(value.Labels)
	value.Annotations = maps.Clone(value.Annotations)
	value.CreatedAt = util.UtcTimePointer(value.CreatedAt)
	value.UpdatedAt = util.UtcTimePointer(value.UpdatedAt)
	return value
}

// ApplyObjectMetaDefaults fills only zero or nil fields. Explicit empty maps
// are preserved, so callers can distinguish "not supplied" from "supplied but
// empty" before or after defaulting.
func ApplyObjectMetaDefaults(
	value, defaults ObjectMeta,
	mode ValidationMode,
	path *common.ValidationContext,
) (ObjectMeta, error) {
	if err := mode.Validate(); err != nil {
		return ObjectMeta{}, validationPath(path).NewError(err.Error())
	}

	if err := validateDefaultsForMode(defaults, mode, path); err != nil {
		return ObjectMeta{}, err
	}

	value = NormalizeObjectMeta(value)
	defaults = NormalizeObjectMeta(defaults)

	// Apply defaults for values that aren't populated
	if value.ID == "" {
		value.ID = defaults.ID
	}
	if value.Name == "" {
		value.Name = defaults.Name
	}
	if value.Namespace == "" {
		value.Namespace = defaults.Namespace
	}
	if value.Generation == 0 {
		value.Generation = defaults.Generation
	}
	if value.Labels == nil {
		value.Labels = defaults.Labels
	}
	if value.Annotations == nil {
		value.Annotations = defaults.Annotations
	}
	if value.CreatedAt == nil {
		value.CreatedAt = defaults.CreatedAt
	}
	if value.UpdatedAt == nil {
		value.UpdatedAt = defaults.UpdatedAt
	}

	return value, nil
}

func validateDefaultsForMode(
	defaults ObjectMeta,
	mode ValidationMode,
	path *common.ValidationContext,
) error {
	vc := validationPath(path).PushField("metadata")

	if mode == ValidationModeCreate {
		if defaults.ID != "" {
			return vc.NewErrorForField("id", "cannot be defaulted in create context")
		}

		if defaults.Generation != 0 {
			return vc.NewErrorForField("generation", "cannot be defaulted in create context")
		}
	}

	if mode == ValidationModeCreate ||
		mode == ValidationModeUpdate ||
		mode == ValidationModeConfig {
		if defaults.CreatedAt != nil {
			return vc.NewErrorForField("createdAt", fmt.Sprintf("cannot be defaulted in %s context", mode))
		}

		if defaults.UpdatedAt != nil {
			return vc.NewErrorForField("updatedAt", fmt.Sprintf("cannot be defaulted in %s context", mode))
		}
	}

	return nil
}

// ApplyObjectMetaPatch applies every non-nil patch field to a deep copy of the
// original metadata. Call ValidateMetadataUpdate and the applicable lifecycle
// validation after applying it so attempts to modify immutable or server-owned
// fields fail instead of being silently discarded.
func ApplyObjectMetaPatch(
	original ObjectMeta,
	patch ObjectMetaPatch,
) ObjectMeta {
	result := NormalizeObjectMeta(original)

	if patch.ID != nil {
		result.ID = *patch.ID
	}
	if patch.Name != nil {
		result.Name = *patch.Name
	}
	if patch.Namespace != nil {
		result.Namespace = *patch.Namespace
	}
	if patch.Generation != nil {
		result.Generation = *patch.Generation
	}
	if patch.Labels != nil {
		result.Labels = maps.Clone(*patch.Labels)
	}
	if patch.Annotations != nil {
		result.Annotations = maps.Clone(*patch.Annotations)
	}
	if patch.CreatedAt != nil {
		result.CreatedAt = util.UtcTimePointer(patch.CreatedAt)
	}
	if patch.UpdatedAt != nil {
		result.UpdatedAt = util.UtcTimePointer(patch.UpdatedAt)
	}

	return result
}
