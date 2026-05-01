package resources

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestExtractLabels_Nil(t *testing.T) {
	var diags diag.Diagnostics
	result := extractLabels(context.Background(), types.MapNull(types.StringType), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestExtractLabels_Unknown(t *testing.T) {
	var diags diag.Diagnostics
	result := extractLabels(context.Background(), types.MapUnknown(types.StringType), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestExtractLabels_Populated(t *testing.T) {
	var diags diag.Diagnostics
	elements := map[string]types.String{
		"env":  types.StringValue("prod"),
		"team": types.StringValue("backend"),
	}
	m, _ := types.MapValueFrom(context.Background(), types.StringType, elements)
	result := extractLabels(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result["env"] != "prod" {
		t.Fatalf("expected env=prod, got env=%s", result["env"])
	}
	if result["team"] != "backend" {
		t.Fatalf("expected team=backend, got team=%s", result["team"])
	}
}

func TestExtractAnnotations_Nil(t *testing.T) {
	var diags diag.Diagnostics
	result := extractAnnotations(context.Background(), types.MapNull(types.StringType), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestExtractAnnotations_Unknown(t *testing.T) {
	var diags diag.Diagnostics
	result := extractAnnotations(context.Background(), types.MapUnknown(types.StringType), &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}

func TestExtractAnnotations_Populated(t *testing.T) {
	var diags diag.Diagnostics
	elements := map[string]types.String{
		"description": types.StringValue("A long description with special chars: !@#$%"),
		"note":        types.StringValue("some note"),
	}
	m, _ := types.MapValueFrom(context.Background(), types.StringType, elements)
	result := extractAnnotations(context.Background(), m, &diags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if result["description"] != "A long description with special chars: !@#$%" {
		t.Fatalf("unexpected description value: %s", result["description"])
	}
	if result["note"] != "some note" {
		t.Fatalf("unexpected note value: %s", result["note"])
	}
}

func TestLabelsToMap_Nil(t *testing.T) {
	result := labelsToMap(nil)
	if !result.IsNull() {
		t.Fatalf("expected null map, got %v", result)
	}
}

func TestLabelsToMap_Empty(t *testing.T) {
	result := labelsToMap(map[string]string{})
	if result.IsNull() {
		t.Fatalf("expected non-null map for empty input")
	}
	elements := result.Elements()
	if len(elements) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(elements))
	}
}

func TestLabelsToMap_Populated(t *testing.T) {
	result := labelsToMap(map[string]string{"env": "prod", "team": "backend"})
	if result.IsNull() {
		t.Fatalf("expected non-null map")
	}
	elements := result.Elements()
	if len(elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elements))
	}
}

func TestLabelsToMap_StripsApxyKeys(t *testing.T) {
	result := labelsToMap(map[string]string{
		"env":            "prod",
		"apxy/ns/-/id":   "root.foo",
		"apxy/ns/-/ns":   "root.foo",
		"apxy/cxr/type":  "google_drive",
	})
	if result.IsNull() {
		t.Fatalf("expected non-null map")
	}
	elements := result.Elements()
	if len(elements) != 1 {
		t.Fatalf("expected 1 element after filtering apxy/ keys, got %d", len(elements))
	}
	if _, hasEnv := elements["env"]; !hasEnv {
		t.Fatalf("expected env key to survive filter, got %v", elements)
	}
}

func TestLabelsToMap_OnlyApxyKeysCollapsesToNull(t *testing.T) {
	result := labelsToMap(map[string]string{
		"apxy/ns/-/id": "root.foo",
		"apxy/ns/-/ns": "root.foo",
	})
	if !result.IsNull() {
		t.Fatalf("expected null map when input contains only apxy/ keys, got %v", result)
	}
}

func TestAnnotationsToMap_Nil(t *testing.T) {
	result := annotationsToMap(nil)
	if !result.IsNull() {
		t.Fatalf("expected null map, got %v", result)
	}
}

func TestAnnotationsToMap_Empty(t *testing.T) {
	result := annotationsToMap(map[string]string{})
	if result.IsNull() {
		t.Fatalf("expected non-null map for empty input")
	}
	elements := result.Elements()
	if len(elements) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(elements))
	}
}

func TestAnnotationsToMap_Populated(t *testing.T) {
	result := annotationsToMap(map[string]string{
		"description": "A long value with special characters: !@#$%^&*()",
		"url":         "https://example.com/path?query=value",
	})
	if result.IsNull() {
		t.Fatalf("expected non-null map")
	}
	elements := result.Elements()
	if len(elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elements))
	}
}
