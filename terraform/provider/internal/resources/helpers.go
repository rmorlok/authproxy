package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// extractLabels converts a types.Map to map[string]string.
func extractLabels(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	result := make(map[string]string)
	diags.Append(m.ElementsAs(ctx, &result, false)...)
	return result
}

// labelsToMap converts map[string]string to a types.Map.
func labelsToMap(labels map[string]string) types.Map {
	if labels == nil {
		return types.MapNull(types.StringType)
	}
	elements := make(map[string]types.String, len(labels))
	for k, v := range labels {
		elements[k] = types.StringValue(v)
	}
	m, _ := types.MapValueFrom(context.Background(), types.StringType, elements)
	return m
}

// pathAttr returns a path.Path for the given attribute name.
func pathAttr(name string) path.Path {
	return path.Root(name)
}
