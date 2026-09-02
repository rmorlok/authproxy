package client

import (
	"encoding/json"
	"testing"
)

func TestCreateRateLimitRequestUsesCanonicalResourceEnvelope(t *testing.T) {
	req := CreateRateLimitRequest{
		APIVersion: RateLimitAPIVersion,
		Kind:       RateLimitKind,
		Metadata: RateLimitMetadata{
			Name:      "salesforce-v3",
			Namespace: "root.acme",
		},
		Spec: RateLimitSpec{
			Scope: &RateLimitScope{ConnectorRef: &ObjectReference{
				APIVersion: RateLimitAPIVersion,
				Kind:       "Connector",
				ID:         "cxr_salesforce",
				Generation: 3,
			}},
			Selector: RateLimitSelector{},
			Bucket:   RateLimitBucket{},
			Algorithm: RateLimitAlgorithm{
				TokenBucket: &RateLimitTokenBucket{Capacity: 60, RefillRate: 1},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if body["apiVersion"] != RateLimitAPIVersion || body["kind"] != RateLimitKind {
		t.Fatalf("type metadata: %s", data)
	}
	if _, exists := body["definition"]; exists {
		t.Fatalf("legacy definition field present: %s", data)
	}
	if _, exists := body["spec"]; !exists {
		t.Fatalf("spec missing: %s", data)
	}
}

func TestUpdateRateLimitRequestEncodesNilScopeAsNull(t *testing.T) {
	req := UpdateRateLimitRequest{
		APIVersion: RateLimitAPIVersion,
		Kind:       RateLimitKind,
		Metadata:   &RateLimitMetadataPatch{},
		Spec:       &RateLimitSpecPatch{},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if scope, exists := body.Spec["scope"]; !exists || scope != nil {
		t.Fatalf("scope must be encoded as null to clear a prior scope: %s", data)
	}
}
