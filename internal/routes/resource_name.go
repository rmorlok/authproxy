package routes

import (
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
)

func optionalResourceName(name *scommon.ResourceName, resourceType string) (scommon.ResourceName, *httperr.Error) {
	if name == nil {
		return "", nil
	}
	if err := name.Validate(); err != nil {
		return "", httperr.BadRequest(fmt.Sprintf("invalid %s name: %s", resourceType, err.Error()), httperr.WithInternalErr(err))
	}
	return *name, nil
}

func resourceNameConflictError(err error, resourceType string, name scommon.ResourceName, namespace string) *httperr.Error {
	if !errors.Is(err, database.ErrDuplicate) {
		return nil
	}
	return httperr.Conflictf("%s name '%s' already exists in namespace '%s'", resourceType, name, namespace)
}
