package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	apiv1 "github.com/rmorlok/authproxy/internal/schema/api/v1alpha1"
	"github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
)

// ListOptions supplies the filters and ordering supported by a resource
// collection. ConnectionID applies only to Tools. Empty values are omitted.
type ListOptions struct {
	Namespace     string
	Name          string
	LabelSelector string
	OrderBy       string
	ConnectionID  string
}

// ListResources reads and validates every page of the resource collection. A
// failed or incomplete read returns an error rather than partial results. The
// server applies the filters, which may include descendant namespaces.
func (c *Client) ListResources(
	ctx context.Context,
	kind meta.Kind,
	options ListOptions,
) ([]*LiveResource, error) {
	descriptor, err := resourceType(kind)
	if err != nil {
		return nil, err
	}

	if options.ConnectionID != "" {
		if kind != tools.ToolKind {
			return nil, fmt.Errorf("connection ID filtering is only supported for Tools")
		}
		if err := connection.ValidateID(options.ConnectionID); err != nil {
			return nil, fmt.Errorf("invalid connection ID filter")
		}
	}

	query := url.Values{}
	for key, value := range map[string]string{
		"namespace":     options.Namespace,
		"name":          options.Name,
		"labelSelector": options.LabelSelector,
		"orderBy":       options.OrderBy,
		"connectionId":  options.ConnectionID,
	} {
		if value != "" {
			query.Set(key, value)
		}
	}

	return c.listResources(ctx, kind, descriptor.Collection, query, func(m meta.ObjectMeta) (string, error) {
		return m.ID, nil
	})
}

// listResources validates a complete collection, using its identity rule to
// detect duplicate items across all pages. Generation lists can repeat the
// logical ID but each generation number must occur only once.
func (c *Client) listResources(
	ctx context.Context,
	kind meta.Kind,
	path string,
	query url.Values,
	identity func(meta.ObjectMeta) (string, error),
) ([]*LiveResource, error) {
	seen := map[string]bool{}
	ids := map[string]bool{}

	var result []*LiveResource
	for {
		data, redacted, err := c.request(ctx, http.MethodGet, path, query, nil)
		if err != nil {
			return nil, err
		}

		var page apiv1.ResourceList[json.RawMessage]
		if util.DecodeJSONStrict(data, &page) != nil ||
			page.APIVersion != meta.APIVersionV1Alpha1 ||
			page.Kind != apiv1.ListKind(kind) ||
			page.Items == nil {
			return nil, fmt.Errorf("invalid resource list response")
		}

		if page.Metadata.RemainingItemCount != nil &&
			*page.Metadata.RemainingItemCount < 0 {
			return nil, fmt.Errorf("invalid resource list count")
		}

		for _, item := range page.Items {
			live, err := c.decodeLive(kind, item, redacted)
			if err != nil {
				return nil, err
			}

			key, err := identity(live.Metadata)
			if err != nil {
				return nil, err
			}
			if ids[key] {
				return nil, fmt.Errorf("duplicate identity in resource list")
			}

			ids[key] = true
			result = append(result, live)
		}

		cursor := page.Metadata.Continue

		if cursor == "" {
			if page.Metadata.RemainingItemCount != nil &&
				*page.Metadata.RemainingItemCount > 0 {
				return nil, fmt.Errorf("incomplete resource list")
			}

			return result, nil
		}

		if seen[cursor] {
			return nil, fmt.Errorf("repeated resource list cursor")
		}

		// The server's cursor carries the filters and sort from the first page.
		seen[cursor] = true
		query = url.Values{"cursor": {cursor}}
	}
}
