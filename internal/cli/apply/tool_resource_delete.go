package apply

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// DeleteToolResource deletes one Tool or ToolSet by ID. The server enforces
// ownership and deletion policy. Return its API status without retrying.
func (c *Client) DeleteToolResource(ctx context.Context, kind meta.Kind, id apid.ID) error {
	if kind != tools.ToolKind && kind != toolsets.ToolSetKind {
		return fmt.Errorf("deletion is only supported for Tool and ToolSet resources")
	}

	descriptor, err := resourceType(kind)
	if err != nil {
		return err
	}
	if err := descriptor.ValidateID(id.String()); err != nil {
		return fmt.Errorf("invalid %s ID", kind)
	}

	path := pathToOptionalGeneration(descriptor.Collection, id.String(), 0)
	_, _, err = c.request(ctx, http.MethodDelete, path, nil, nil)
	return err
}
