package datasources

import (
	"testing"
)

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
		"env":           "prod",
		"apxy/ns/-/id":  "root.foo",
		"apxy/cxr/type": "google_drive",
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
