package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

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
