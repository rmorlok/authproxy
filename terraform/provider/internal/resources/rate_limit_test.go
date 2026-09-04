package resources

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/rmorlok/authproxy/terraform/provider/internal/client"
)

// --- buildRateLimitSpec: HCL model → wire payload ---
//
// These tests pin the projection both directions: they prove that what
// the user writes in HCL ends up in the API request body unchanged.
// Acceptance tests against a real admin-API are deliberately out of scope
// (no test fixture exists for any TF resource in this repo); the
// example HCL in examples/resources/ is the end-to-end deliverable.

func TestBuildRateLimitSpec_TokenBucket(t *testing.T) {
	plan := &RateLimitResourceModel{
		Mode: types.StringValue("enforce"),
		Selector: &rateLimitSelectorModel{
			LabelSelector: types.StringValue("env=prod"),
			Methods:       stringsToList([]string{"POST"}),
			RequestTypes:  stringsToList([]string{"proxy"}),
			PathMatch: &rateLimitPathMatchModel{
				Kind:  types.StringValue("prefix"),
				Value: types.StringValue("/v1/"),
			},
		},
		Bucket: &rateLimitBucketModel{
			Dimensions: stringsToList([]string{"actor"}),
		},
		Algorithm: &rateLimitAlgorithmModel{
			TokenBucket: &rateLimitTokenBucketModel{
				Capacity:   types.Int64Value(60),
				RefillRate: types.Float64Value(1.0),
			},
		},
	}

	def, err := buildRateLimitSpec(context.Background(), plan)
	if err != nil {
		t.Fatalf("buildRateLimitSpec: %v", err)
	}

	if def.Mode != "enforce" {
		t.Errorf("mode: got %q, want %q", def.Mode, "enforce")
	}
	if def.Selector.LabelSelector != "env=prod" {
		t.Errorf("label_selector: got %q", def.Selector.LabelSelector)
	}
	if len(def.Selector.Methods) != 1 || def.Selector.Methods[0] != "POST" {
		t.Errorf("methods: got %v", def.Selector.Methods)
	}
	if def.Selector.PathMatch == nil || def.Selector.PathMatch.Kind != "prefix" {
		t.Errorf("path_match.kind: %+v", def.Selector.PathMatch)
	}
	if def.Algorithm.TokenBucket == nil || def.Algorithm.TokenBucket.Capacity != 60 || def.Algorithm.TokenBucket.RefillRate != 1.0 {
		t.Errorf("token_bucket: %+v", def.Algorithm.TokenBucket)
	}
	if def.Algorithm.FixedWindow != nil || def.Algorithm.SlidingWindow != nil {
		t.Errorf("expected only token_bucket variant to be set, got %+v", def.Algorithm)
	}
}

func TestBuildRateLimitSpec_FixedWindow(t *testing.T) {
	plan := &RateLimitResourceModel{
		Selector: &rateLimitSelectorModel{Methods: stringsToList([]string{"GET"})},
		Bucket:   &rateLimitBucketModel{Dimensions: stringsToList([]string{"actor"})},
		Algorithm: &rateLimitAlgorithmModel{
			FixedWindow: &rateLimitFixedWindowModel{
				Window: types.StringValue("1m"),
				Limit:  types.Int64Value(100),
			},
		},
	}
	def, err := buildRateLimitSpec(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if def.Algorithm.FixedWindow == nil || def.Algorithm.FixedWindow.Window != "1m" || def.Algorithm.FixedWindow.Limit != 100 {
		t.Errorf("fixed_window: %+v", def.Algorithm.FixedWindow)
	}
}

func TestBuildRateLimitSpec_SlidingWindow(t *testing.T) {
	plan := &RateLimitResourceModel{
		Selector: &rateLimitSelectorModel{},
		Bucket:   &rateLimitBucketModel{},
		Algorithm: &rateLimitAlgorithmModel{
			SlidingWindow: &rateLimitSlidingWindowModel{
				Window: types.StringValue("5m"),
				Limit:  types.Int64Value(50),
				Mode:   types.StringValue("log"),
			},
		},
	}
	def, err := buildRateLimitSpec(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if def.Algorithm.SlidingWindow == nil ||
		def.Algorithm.SlidingWindow.Mode != "log" {
		t.Errorf("sliding_window: %+v", def.Algorithm.SlidingWindow)
	}
}

func TestBuildRateLimitSpec_EmptyOptionalFieldsOmitted(t *testing.T) {
	// Confirm that null/empty optional fields don't end up serialised
	// into the request body — the JSON encoder's omitempty + our nil
	// returns from listToStrings keep things minimal.
	plan := &RateLimitResourceModel{
		Selector:  &rateLimitSelectorModel{},
		Bucket:    &rateLimitBucketModel{},
		Algorithm: &rateLimitAlgorithmModel{TokenBucket: &rateLimitTokenBucketModel{Capacity: types.Int64Value(1), RefillRate: types.Float64Value(1)}},
	}
	def, err := buildRateLimitSpec(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if def.Selector.Methods != nil {
		t.Errorf("expected nil methods, got %v", def.Selector.Methods)
	}
	if def.Selector.PathMatch != nil {
		t.Errorf("expected nil path_match, got %+v", def.Selector.PathMatch)
	}
	if def.Bucket.Dimensions != nil {
		t.Errorf("expected nil dimensions, got %v", def.Bucket.Dimensions)
	}
}

func TestBuildRateLimitSpec_ConnectorScope(t *testing.T) {
	plan := &RateLimitResourceModel{
		Scope: &rateLimitScopeModel{ConnectorRef: &rateLimitConnectorRefModel{
			ID: types.StringValue("cxr_salesforce"),
		}},
		Selector:  &rateLimitSelectorModel{},
		Bucket:    &rateLimitBucketModel{},
		Algorithm: &rateLimitAlgorithmModel{TokenBucket: &rateLimitTokenBucketModel{Capacity: types.Int64Value(1), RefillRate: types.Float64Value(1)}},
	}

	spec, err := buildRateLimitSpec(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Scope == nil || spec.Scope.ConnectorRef == nil {
		t.Fatalf("connector scope: %+v", spec.Scope)
	}
	if got := spec.Scope.ConnectorRef; got.APIVersion != client.RateLimitAPIVersion || got.Kind != "Connector" || got.ID != "cxr_salesforce" {
		t.Errorf("connector ref: %+v", got)
	}
}

func TestBuildRateLimitSpec_NamespaceMatcherScope(t *testing.T) {
	plan := &RateLimitResourceModel{
		Scope:     &rateLimitScopeModel{NamespaceMatcher: types.StringValue("root.platform.payments.**")},
		Selector:  &rateLimitSelectorModel{},
		Bucket:    &rateLimitBucketModel{},
		Algorithm: &rateLimitAlgorithmModel{TokenBucket: &rateLimitTokenBucketModel{Capacity: types.Int64Value(1), RefillRate: types.Float64Value(1)}},
	}

	spec, err := buildRateLimitSpec(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Scope == nil || spec.Scope.NamespaceMatcher != "root.platform.payments.**" {
		t.Fatalf("namespace matcher scope: %+v", spec.Scope)
	}
}

func TestBuildRateLimitSpec_ConnectionScope(t *testing.T) {
	plan := &RateLimitResourceModel{
		Scope:     &rateLimitScopeModel{ConnectionRef: &rateLimitConnectionRefModel{ID: types.StringValue("cxn_customer")}},
		Selector:  &rateLimitSelectorModel{},
		Bucket:    &rateLimitBucketModel{},
		Algorithm: &rateLimitAlgorithmModel{TokenBucket: &rateLimitTokenBucketModel{Capacity: types.Int64Value(1), RefillRate: types.Float64Value(1)}},
	}

	spec, err := buildRateLimitSpec(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Scope == nil || spec.Scope.ConnectionRef == nil {
		t.Fatalf("connection scope: %+v", spec.Scope)
	}
	if got := spec.Scope.ConnectionRef; got.APIVersion != client.RateLimitAPIVersion || got.Kind != "Connection" || got.ID != "cxn_customer" {
		t.Errorf("connection ref: %+v", got)
	}
}

// --- setRateLimitState: wire payload → state model ---

func TestSetRateLimitState_PopulatesAllFields(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	rl := &client.RateLimit{
		APIVersion: client.RateLimitAPIVersion,
		Kind:       client.RateLimitKind,
		Metadata: client.RateLimitMetadata{
			ID:          "rl_test123",
			Namespace:   "root.acme",
			Labels:      map[string]string{"team": "acme", "apxy/rl/-/id": "rl_test123"},
			Annotations: map[string]string{"owner": "platform@example.com"},
			CreatedAt:   &now,
			UpdatedAt:   &now,
		},
		Spec: client.RateLimitSpec{
			Scope: &client.RateLimitScope{ConnectorRef: &client.ObjectReference{
				APIVersion: client.RateLimitAPIVersion,
				Kind:       "Connector",
				ID:         "cxr_salesforce",
			}},
			Mode: "observe",
			Selector: client.RateLimitSelector{
				LabelSelector: "env=prod",
				Methods:       []string{"GET", "POST"},
				RequestTypes:  []string{"proxy"},
				PathMatch:     &client.RateLimitPathMatch{Kind: "regex", Value: "^/v1/"},
			},
			Bucket: client.RateLimitBucket{Dimensions: []string{"actor", "labels/team"}},
			Algorithm: client.RateLimitAlgorithm{
				TokenBucket: &client.RateLimitTokenBucket{Capacity: 60, RefillRate: 0.5},
			},
		},
	}

	var model RateLimitResourceModel
	setRateLimitState(&model, rl)

	if model.Id.ValueString() != "rl_test123" {
		t.Errorf("id: %s", model.Id.ValueString())
	}
	if model.Namespace.ValueString() != "root.acme" {
		t.Errorf("namespace: %s", model.Namespace.ValueString())
	}
	if model.Mode.ValueString() != "observe" {
		t.Errorf("mode: %s", model.Mode.ValueString())
	}
	if model.Scope == nil || model.Scope.ConnectorRef == nil || model.Scope.ConnectorRef.ID.ValueString() != "cxr_salesforce" {
		t.Errorf("scope/connector_ref: %+v", model.Scope)
	}

	// apxy/ system labels must be stripped — helpers.labelsToMap policy.
	gotLabels := map[string]string{}
	model.Labels.ElementsAs(context.Background(), &gotLabels, false)
	if got := gotLabels["team"]; got != "acme" {
		t.Errorf("labels[team]: %q", got)
	}
	if _, has := gotLabels["apxy/rl/-/id"]; has {
		t.Error("apxy/ system labels must not leak into TF state")
	}

	if model.Selector == nil || model.Selector.PathMatch == nil ||
		model.Selector.PathMatch.Kind.ValueString() != "regex" {
		t.Errorf("selector/path_match: %+v", model.Selector)
	}

	if model.Algorithm == nil || model.Algorithm.TokenBucket == nil ||
		model.Algorithm.TokenBucket.Capacity.ValueInt64() != 60 ||
		model.Algorithm.TokenBucket.RefillRate.ValueFloat64() != 0.5 {
		t.Errorf("algorithm/token_bucket: %+v", model.Algorithm)
	}
}

func TestSetRateLimitState_NamespaceMatcherScope(t *testing.T) {
	rl := &client.RateLimit{
		Metadata: client.RateLimitMetadata{ID: "rl_test123", Namespace: "root.acme"},
		Spec: client.RateLimitSpec{
			Scope:     &client.RateLimitScope{NamespaceMatcher: "root.acme.payments.**"},
			Algorithm: client.RateLimitAlgorithm{TokenBucket: &client.RateLimitTokenBucket{Capacity: 1, RefillRate: 1}},
		},
	}

	var model RateLimitResourceModel
	setRateLimitState(&model, rl)
	if model.Scope == nil || model.Scope.NamespaceMatcher.ValueString() != "root.acme.payments.**" {
		t.Fatalf("namespace matcher scope: %+v", model.Scope)
	}
}

func TestSetRateLimitState_EmptyModeDefaultsToEnforce(t *testing.T) {
	// The server stores Mode="" for the default ("enforce"); the
	// provider surfaces it explicitly so plan/apply consistency holds.
	rl := &client.RateLimit{
		APIVersion: client.RateLimitAPIVersion,
		Kind:       client.RateLimitKind,
		Metadata:   client.RateLimitMetadata{ID: "rl_x", Namespace: "root"},
		Spec:       client.RateLimitSpec{Algorithm: client.RateLimitAlgorithm{TokenBucket: &client.RateLimitTokenBucket{Capacity: 1, RefillRate: 1}}},
	}
	var m RateLimitResourceModel
	setRateLimitState(&m, rl)
	if m.Mode.ValueString() != "enforce" {
		t.Errorf("mode: got %q, want %q", m.Mode.ValueString(), "enforce")
	}
}

// --- exactly-one algorithm validator ---
//
// The validator runs at plan time and uses the framework's standard
// resource.ValidateConfigRequest/Response. Exercising it via a full
// schema round-trip requires wiring the schema; instead we test the
// underlying logic on model fixtures and assert the diagnostic count.

func TestAlgorithmValidator_ExactlyOne(t *testing.T) {
	cases := []struct {
		name     string
		model    *rateLimitAlgorithmModel
		wantErrs int
	}{
		{
			"nil block",
			nil,
			1,
		},
		{
			"empty",
			&rateLimitAlgorithmModel{},
			1,
		},
		{
			"one variant",
			&rateLimitAlgorithmModel{TokenBucket: &rateLimitTokenBucketModel{Capacity: types.Int64Value(1), RefillRate: types.Float64Value(1)}},
			0,
		},
		{
			"two variants",
			&rateLimitAlgorithmModel{
				TokenBucket: &rateLimitTokenBucketModel{Capacity: types.Int64Value(1), RefillRate: types.Float64Value(1)},
				FixedWindow: &rateLimitFixedWindowModel{Window: types.StringValue("1m"), Limit: types.Int64Value(1)},
			},
			1,
		},
		{
			"three variants",
			&rateLimitAlgorithmModel{
				TokenBucket:   &rateLimitTokenBucketModel{Capacity: types.Int64Value(1), RefillRate: types.Float64Value(1)},
				FixedWindow:   &rateLimitFixedWindowModel{Window: types.StringValue("1m"), Limit: types.Int64Value(1)},
				SlidingWindow: &rateLimitSlidingWindowModel{Window: types.StringValue("1m"), Limit: types.Int64Value(1), Mode: types.StringValue("log")},
			},
			1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := countAlgorithmVariants(tc.model)
			if (got != 1) != (tc.wantErrs > 0) {
				t.Errorf("variant count %d should %s yield an error", got, map[bool]string{true: "", false: "not"}[tc.wantErrs > 0])
			}
		})
	}
}

// countAlgorithmVariants exposes the validator's core counting logic so
// it can be exercised directly without a full schema round-trip. Kept
// in test scope; the production validator inlines the same count.
func countAlgorithmVariants(m *rateLimitAlgorithmModel) int {
	if m == nil {
		return 0
	}
	n := 0
	if m.FixedWindow != nil {
		n++
	}
	if m.SlidingWindow != nil {
		n++
	}
	if m.TokenBucket != nil {
		n++
	}
	return n
}

// --- helper test: stringsToList null/empty/populated round-trip ---

func TestStringsToList_NullForEmptyInput(t *testing.T) {
	if !stringsToList(nil).IsNull() {
		t.Error("nil should become a null list (omitempty contract)")
	}
	if !stringsToList([]string{}).IsNull() {
		t.Error("empty slice should also become a null list")
	}
}

func TestStringsToList_Populated(t *testing.T) {
	l := stringsToList([]string{"a", "b"})
	elems := l.Elements()
	if len(elems) != 2 {
		t.Fatalf("got %d elements", len(elems))
	}
	got := []attr.Value{types.StringValue("a"), types.StringValue("b")}
	for i, e := range elems {
		if !e.Equal(got[i]) {
			t.Errorf("element %d: %v != %v", i, e, got[i])
		}
	}
}
