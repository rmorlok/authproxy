package meta

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
)

// ValidationMode identifies the source or lifecycle operation whose metadata
// policy is being applied.
type ValidationMode string

const (
	ValidationModeCreate      ValidationMode = "create"
	ValidationModeUpdate      ValidationMode = "update"
	ValidationModeConfig      ValidationMode = "config"
	ValidationModePersistence ValidationMode = "persistence"
	ValidationModeResponse    ValidationMode = "response"
)

func (m ValidationMode) Validate() error {
	switch m {
	case ValidationModeCreate, ValidationModeUpdate, ValidationModeConfig, ValidationModePersistence, ValidationModeResponse:
		return nil
	default:
		return fmt.Errorf("unknown metadata validation mode %q", m)
	}
}

// ValidationOptions supplies resource-specific requirements while retaining a
// single shared implementation for lifecycle and metadata rules.
type ValidationOptions struct {
	Mode               ValidationMode
	Path               *common.ValidationContext
	ExpectedAPIVersion APIVersion
	ExpectedKind       Kind
	RequireID          bool
	RequireName        bool
	RequireNamespace   bool
	IDValidator        func(string) error
	NamespaceValidator func(string) error
}

// ObjectReferenceValidationOptions supplies target-specific requirements while
// retaining the shared ID-or-namespaced-name reference convention.
type ObjectReferenceValidationOptions struct {
	ExpectedAPIVersion APIVersion
	ExpectedKind       Kind
	IDValidator        func(string) error
	NamespaceValidator func(string) error
}

// UpdateOptions selects the resource-specific metadata fields that are
// immutable in addition to ID, generation, and createdAt.
type UpdateOptions struct {
	ImmutableName      bool
	ImmutableNamespace bool
}

// validationPath returns a validation context for the given path. It defaults
// to the root path if no path is provided.
func validationPath(
	path *common.ValidationContext,
) *common.ValidationContext {
	if path == nil {
		return &common.ValidationContext{Path: "$"}
	}

	return path
}

// ValidateTypeMeta validates that TypeMeta has the expected APIVersion and
// Kind. It optionally validates those values are specific values.
func ValidateTypeMeta(
	value TypeMeta,
	expectedVersion APIVersion, // optional if specific version is expected
	expectedKind Kind, // optional if specific kind is expected
	path *common.ValidationContext,
) error {
	vc := validationPath(path)
	var result *multierror.Error

	// APIVersion must be present, valid format, and match expectations if
	// expectations are specified.
	if value.APIVersion == "" {
		result = multierror.Append(result, vc.NewErrorForField("apiVersion", "is required"))
	} else if err := value.APIVersion.Validate(); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("apiVersion", "%v", err))
	} else if expectedVersion != "" && value.APIVersion != expectedVersion {
		result = multierror.Append(result, vc.NewErrorfForField("apiVersion", "must be %q, got %q", expectedVersion, value.APIVersion))
	}

	// Kind must be present, valid format and match expectations if expectations
	// are specified.
	if value.Kind == "" {
		result = multierror.Append(result, vc.NewErrorForField("kind", "is required"))
	} else if err := value.Kind.Validate(); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("kind", "%v", err))
	} else if expectedKind != "" && value.Kind != expectedKind {
		result = multierror.Append(result, vc.NewErrorfForField("kind", "must be %q, got %q", expectedKind, value.Kind))
	}

	return result.ErrorOrNil()
}

// ValidateStatus enforces the shared rule that status is server-owned on API
// and configuration writes. Resource packages remain responsible for the
// contents of their status types.
func ValidateStatus(
	value any,
	mode ValidationMode,
	path *common.ValidationContext,
) error {
	if err := mode.Validate(); err != nil {
		return validationPath(path).NewError(err.Error())
	}

	if mode == ValidationModePersistence ||
		mode == ValidationModeResponse ||
		util.IsZeroValue(value) {
		return nil
	}

	return validationPath(path).
		NewErrorForField("status", "is server-owned")
}

func ValidateObjectReference(
	value ObjectReference,
	path *common.ValidationContext,
) error {
	return ValidateObjectReferenceWithOptions(value, ObjectReferenceValidationOptions{}, path)
}

// ValidateObjectReferenceWithOptions validates the common object-reference
// shape and any target-specific GVK, ID, or namespace requirements. References
// may use an immutable ID or a namespace and name pair.
func ValidateObjectReferenceWithOptions(
	value ObjectReference,
	options ObjectReferenceValidationOptions,
	path *common.ValidationContext,
) error {
	vc := validationPath(path)
	var result *multierror.Error

	if err := ValidateTypeMeta(
		TypeMeta{
			APIVersion: value.APIVersion,
			Kind:       value.Kind,
		},
		options.ExpectedAPIVersion,
		options.ExpectedKind,
		vc,
	); err != nil {
		result = multierror.Append(result, err)
	}

	if !value.HasID() && !value.HasNamespacedName() {
		result = multierror.Append(result, vc.NewError("must contain id or namespace and name"))
	}

	if value.ID != "" && options.IDValidator != nil {
		if err := options.IDValidator(value.ID); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("id", "%v", err))
		}
	}

	if value.Name != "" {
		if err := value.Name.Validate(); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("name", "%v", err))
		}
	}

	if value.Namespace != "" && options.NamespaceValidator != nil {
		if err := options.NamespaceValidator(value.Namespace); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("namespace", "%v", err))
		}
	}

	return result.ErrorOrNil()
}

func ValidateCondition(value Condition, path *common.ValidationContext) error {
	vc := validationPath(path)
	var result *multierror.Error

	if value.Type == "" {
		result = multierror.Append(result, vc.NewErrorForField("type", "is required"))
	}

	switch value.Status {
	case ConditionTrue, ConditionFalse, ConditionUnknown:
	default:
		result = multierror.Append(result, vc.NewErrorfForField("status", "must be one of %q, %q, or %q", ConditionTrue, ConditionFalse, ConditionUnknown))
	}

	if value.LastTransitionTime.IsZero() {
		result = multierror.Append(result, vc.NewErrorForField("lastTransitionTime", "is required"))
	}

	return result.ErrorOrNil()
}

func ValidateObjectMeta(value ObjectMeta, options ValidationOptions) error {
	vc := validationPath(options.Path).PushField("metadata")
	var result *multierror.Error

	if err := options.Mode.Validate(); err != nil {
		result = multierror.Append(result, validationPath(options.Path).NewError(err.Error()))
	}

	if options.RequireID && value.ID == "" {
		result = multierror.Append(result, vc.NewErrorForField("id", "is required"))
	}

	if value.ID != "" && options.IDValidator != nil {
		if err := options.IDValidator(value.ID); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("id", "%v", err))
		}
	}

	if options.RequireName && value.Name == "" {
		result = multierror.Append(result, vc.NewErrorForField("name", "is required"))
	} else if value.Name != "" {
		if err := value.Name.Validate(); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("name", "%v", err))
		}
	}

	if options.RequireNamespace && value.Namespace == "" {
		result = multierror.Append(result, vc.NewErrorForField("namespace", "is required"))
	} else if value.Namespace != "" && options.NamespaceValidator != nil {
		if err := options.NamespaceValidator(value.Namespace); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("namespace", "%v", err))
		}
	}

	switch options.Mode {
	case ValidationModeCreate:
		if value.ID != "" {
			result = multierror.Append(result, vc.NewErrorForField("id", "is server-owned on create"))
		}
		if value.Generation != 0 {
			result = multierror.Append(result, vc.NewErrorForField("generation", "is server-owned on create"))
		}
		result = appendTimestampWriteErrors(result, vc, value)
	case ValidationModeUpdate:
		result = appendTimestampWriteErrors(result, vc, value)
	case ValidationModeConfig:
		result = appendTimestampWriteErrors(result, vc, value)
	}

	labelValidator := ValidateUserLabels
	if options.Mode == ValidationModePersistence || options.Mode == ValidationModeResponse {
		labelValidator = ValidateLabels
	}

	if err := labelValidator(value.Labels); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("labels", "%v", err))
	}

	if err := ValidateAnnotations(value.Annotations); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("annotations", "%v", err))
	}

	return result.ErrorOrNil()
}

// ValidateObjectMetaPatch validates the fields that are present in a metadata
// patch. Resource packages supply validators for their ID and namespace
// formats, just as they do for complete ObjectMeta values.
func ValidateObjectMetaPatch(value ObjectMetaPatch, options ValidationOptions) error {
	vc := validationPath(options.Path).PushField("metadata")
	var result *multierror.Error

	if options.Mode != ValidationModeUpdate {
		result = multierror.Append(result, validationPath(options.Path).
			NewErrorf("metadata patches require update validation mode, got %q", options.Mode))
	}

	if value.ID != nil {
		if *value.ID == "" {
			result = multierror.Append(result, vc.NewErrorForField("id", "must not be empty when provided"))
		} else if options.IDValidator != nil {
			if err := options.IDValidator(*value.ID); err != nil {
				result = multierror.Append(result, vc.NewErrorfForField("id", "%v", err))
			}
		}
	}

	if value.Name != nil {
		if err := value.Name.Validate(); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("name", "%v", err))
		}
	}

	if value.Namespace != nil {
		if *value.Namespace == "" {
			result = multierror.Append(result, vc.NewErrorForField("namespace", "must not be empty when provided"))
		} else if options.NamespaceValidator != nil {
			if err := options.NamespaceValidator(*value.Namespace); err != nil {
				result = multierror.Append(result, vc.NewErrorfForField("namespace", "%v", err))
			}
		}
	}

	if value.Generation != nil && *value.Generation == 0 {
		result = multierror.Append(result, vc.NewErrorForField("generation", "must be greater than zero when provided"))
	}

	if value.Labels != nil {
		if err := ValidateUserLabels(*value.Labels); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("labels", "%v", err))
		}
	}
	if value.Annotations != nil {
		if err := ValidateAnnotations(*value.Annotations); err != nil {
			result = multierror.Append(result, vc.NewErrorfForField("annotations", "%v", err))
		}
	}

	if value.CreatedAt != nil {
		result = multierror.Append(result, vc.NewErrorForField("createdAt", "is server-owned"))
	}
	if value.UpdatedAt != nil {
		result = multierror.Append(result, vc.NewErrorForField("updatedAt", "is server-owned"))
	}

	return result.ErrorOrNil()
}

func appendTimestampWriteErrors(
	result *multierror.Error,
	vc *common.ValidationContext,
	value ObjectMeta,
) *multierror.Error {
	if value.CreatedAt != nil {
		result = multierror.Append(result, vc.NewErrorForField("createdAt", "is server-owned"))
	}

	if value.UpdatedAt != nil {
		result = multierror.Append(result, vc.NewErrorForField("updatedAt", "is server-owned"))
	}

	return result
}

func ValidateResource(
	value TypeMeta,
	metadata ObjectMeta,
	options ValidationOptions,
) error {
	var result *multierror.Error

	if err := ValidateTypeMeta(
		value,
		options.ExpectedAPIVersion,
		options.ExpectedKind,
		options.Path,
	); err != nil {
		result = multierror.Append(result, err)
	}

	if err := ValidateObjectMeta(metadata, options); err != nil {
		result = multierror.Append(result, err)
	}

	return result.ErrorOrNil()
}

func ValidateMetadataUpdate(before, after ObjectMeta, options UpdateOptions, path *common.ValidationContext) error {
	vc := validationPath(path).PushField("metadata")
	var result *multierror.Error

	if before.ID != after.ID {
		result = multierror.Append(result, vc.NewErrorForField("id", "is immutable"))
	}

	if before.Generation != after.Generation {
		result = multierror.Append(result, vc.NewErrorForField("generation", "is immutable"))
	}

	if !util.EqualTimePointers(before.CreatedAt, after.CreatedAt) {
		result = multierror.Append(result, vc.NewErrorForField("createdAt", "is immutable"))
	}

	if options.ImmutableName && before.Name != after.Name {
		result = multierror.Append(result, vc.NewErrorForField("name", "is immutable"))
	}

	if options.ImmutableNamespace && before.Namespace != after.Namespace {
		result = multierror.Append(result, vc.NewErrorForField("namespace", "is immutable"))
	}

	return result.ErrorOrNil()
}

func ValidateTypeMetaUpdate(
	before, after TypeMeta,
	path *common.ValidationContext,
) error {
	vc := validationPath(path)
	var result *multierror.Error
	if before.APIVersion != after.APIVersion {
		result = multierror.Append(result, vc.NewErrorForField("apiVersion", "is immutable"))
	}
	if before.Kind != after.Kind {
		result = multierror.Append(result, vc.NewErrorForField("kind", "is immutable"))
	}
	return result.ErrorOrNil()
}
