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
