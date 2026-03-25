package datasources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

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
