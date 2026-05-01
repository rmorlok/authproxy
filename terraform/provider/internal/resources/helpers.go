package resources

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// apxyReservedLabelPrefix is the system-managed label-key namespace.
// Labels under this prefix are populated by the AuthProxy server (e.g.
// implicit identifier labels and parent carry-forward labels) and are not
// part of the user-managed configuration. The provider strips them when
// reading labels back from the API so Terraform's plan/apply consistency
// check doesn't see them as unexpected new elements.
const apxyReservedLabelPrefix = "apxy/"

// extractLabels converts a types.Map to map[string]string.
func extractLabels(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	result := make(map[string]string)
	diags.Append(m.ElementsAs(ctx, &result, false)...)
	return result
}

// labelsToMap converts map[string]string to a types.Map. System-managed
// labels under the apxy/ namespace are stripped — they are not part of the
// user-managed Terraform configuration. If the API response contains only
// apxy/ entries (so the filtered result is empty), the result collapses to
// MapNull so resources where the user did not configure any labels stay
// null in state. A genuinely empty input (no entries at all) is preserved
// as a non-null empty map.
func labelsToMap(labels map[string]string) types.Map {
	if labels == nil {
		return types.MapNull(types.StringType)
	}
	if len(labels) == 0 {
		m, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]types.String{})
		return m
	}
	elements := make(map[string]types.String, len(labels))
	for k, v := range labels {
		if strings.HasPrefix(k, apxyReservedLabelPrefix) {
			continue
		}
		elements[k] = types.StringValue(v)
	}
	if len(elements) == 0 {
		return types.MapNull(types.StringType)
	}
	m, _ := types.MapValueFrom(context.Background(), types.StringType, elements)
	return m
}

// extractAnnotations converts a types.Map to map[string]string.
func extractAnnotations(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	result := make(map[string]string)
	diags.Append(m.ElementsAs(ctx, &result, false)...)
	return result
}

// annotationsToMap converts map[string]string to a types.Map.
func annotationsToMap(annotations map[string]string) types.Map {
	if annotations == nil {
		return types.MapNull(types.StringType)
	}
	elements := make(map[string]types.String, len(annotations))
	for k, v := range annotations {
		elements[k] = types.StringValue(v)
	}
	m, _ := types.MapValueFrom(context.Background(), types.StringType, elements)
	return m
}

// pathAttr returns a path.Path for the given attribute name.
func pathAttr(name string) path.Path {
	return path.Root(name)
}
