package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		expected LabelSelector
		wantErr  bool
	}{
		{
			name:     "empty selector",
			selector: "",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "single equality",
			selector: "app=web",
			expected: LabelSelector{
				{Key: "app", Operator: LabelOperatorEqual, Value: "web"},
			},
			wantErr: false,
		},
		{
			name:     "single double equality",
			selector: "app==web",
			expected: LabelSelector{
				{Key: "app", Operator: LabelOperatorEqual, Value: "web"},
			},
			wantErr: false,
		},
		{
			name:     "single inequality",
			selector: "env!=prod",
			expected: LabelSelector{
				{Key: "env", Operator: LabelOperatorNotEqual, Value: "prod"},
			},
			wantErr: false,
		},
		{
			name:     "exists",
			selector: "tier",
			expected: LabelSelector{
				{Key: "tier", Operator: LabelOperatorExists},
			},
			wantErr: false,
		},
		{
			name:     "not exists",
			selector: "!tier",
			expected: LabelSelector{
				{Key: "tier", Operator: LabelOperatorNotExists},
			},
			wantErr: false,
		},
		{
			name:     "multiple",
			selector: "app=web,env!=prod,tier,!deprecated",
			expected: LabelSelector{
				{Key: "app", Operator: LabelOperatorEqual, Value: "web"},
				{Key: "env", Operator: LabelOperatorNotEqual, Value: "prod"},
				{Key: "tier", Operator: LabelOperatorExists},
				{Key: "deprecated", Operator: LabelOperatorNotExists},
			},
			wantErr: false,
		},
		{
			name:     "with spaces",
			selector: " app = web , env != prod ",
			expected: LabelSelector{
				{Key: "app", Operator: LabelOperatorEqual, Value: "web"},
				{Key: "env", Operator: LabelOperatorNotEqual, Value: "prod"},
			},
			wantErr: false,
		},
		{
			name:     "invalid key",
			selector: "invalid key=val",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid value",
			selector: "key=invalid value",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLabelSelector(tt.selector)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestLabelSelector_String(t *testing.T) {
	selector := LabelSelector{
		{Key: "app", Operator: LabelOperatorEqual, Value: "web"},
		{Key: "env", Operator: LabelOperatorNotEqual, Value: "prod"},
		{Key: "tier", Operator: LabelOperatorExists},
		{Key: "deprecated", Operator: LabelOperatorNotExists},
	}
	expected := "app=web,env!=prod,tier,!deprecated"
	assert.Equal(t, expected, selector.String())
}

func TestBuildLabelSelectorFromMap(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "nil map",
			labels:   nil,
			expected: "",
		},
		{
			name:     "empty map",
			labels:   map[string]string{},
			expected: "",
		},
		{
			name:     "single label",
			labels:   map[string]string{"type": "salesforce"},
			expected: "type=salesforce",
		},
		{
			name:     "multiple labels sorted alphabetically",
			labels:   map[string]string{"type": "salesforce", "env": "prod"},
			expected: "env=prod,type=salesforce",
		},
		{
			name:     "three labels sorted",
			labels:   map[string]string{"type": "gmail", "env": "staging", "app": "email"},
			expected: "app=email,env=staging,type=gmail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildLabelSelectorFromMap(tt.labels)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestBuildLabelSelectorFromMap_Roundtrip(t *testing.T) {
	// Test that BuildLabelSelectorFromMap output can be parsed back
	labels := map[string]string{"type": "salesforce", "env": "prod", "tier": "backend"}
	selector := BuildLabelSelectorFromMap(labels)

	parsed, err := ParseLabelSelector(selector)
	assert.NoError(t, err)
	assert.Len(t, parsed, 3)

	// Verify all requirements are equality checks with correct values
	for _, req := range parsed {
		assert.Equal(t, LabelOperatorEqual, req.Operator)
		assert.Equal(t, labels[req.Key], req.Value)
	}
}

// TestLabelSelector_Matches covers the in-memory evaluator added for the
// rate-limit runtime. Each case exercises one operator + a few of the
// edge interactions that the SQL emitter covers via predicates.
func TestLabelSelector_Matches(t *testing.T) {
	cases := []struct {
		name     string
		selector LabelSelector
		labels   map[string]string
		want     bool
	}{
		{"empty selector matches anything", nil, map[string]string{"x": "y"}, true},
		{"empty selector matches nil labels", nil, nil, true},
		{
			"= matches",
			LabelSelector{{Key: "env", Operator: LabelOperatorEqual, Value: "prod"}},
			map[string]string{"env": "prod"},
			true,
		},
		{
			"= rejects different value",
			LabelSelector{{Key: "env", Operator: LabelOperatorEqual, Value: "prod"}},
			map[string]string{"env": "staging"},
			false,
		},
		{
			"= rejects missing key",
			LabelSelector{{Key: "env", Operator: LabelOperatorEqual, Value: "prod"}},
			map[string]string{},
			false,
		},
		{
			"!= matches different value",
			LabelSelector{{Key: "env", Operator: LabelOperatorNotEqual, Value: "prod"}},
			map[string]string{"env": "staging"},
			true,
		},
		{
			"!= matches missing key",
			LabelSelector{{Key: "env", Operator: LabelOperatorNotEqual, Value: "prod"}},
			map[string]string{},
			true,
		},
		{
			"!= rejects same value",
			LabelSelector{{Key: "env", Operator: LabelOperatorNotEqual, Value: "prod"}},
			map[string]string{"env": "prod"},
			false,
		},
		{
			"exists matches present key",
			LabelSelector{{Key: "team", Operator: LabelOperatorExists}},
			map[string]string{"team": "platform"},
			true,
		},
		{
			"exists matches present key with empty value",
			LabelSelector{{Key: "team", Operator: LabelOperatorExists}},
			map[string]string{"team": ""},
			true,
		},
		{
			"exists rejects missing key",
			LabelSelector{{Key: "team", Operator: LabelOperatorExists}},
			map[string]string{},
			false,
		},
		{
			"!exists rejects present key",
			LabelSelector{{Key: "debug", Operator: LabelOperatorNotExists}},
			map[string]string{"debug": "true"},
			false,
		},
		{
			"!exists matches missing key",
			LabelSelector{{Key: "debug", Operator: LabelOperatorNotExists}},
			map[string]string{},
			true,
		},
		{
			"all clauses ANDed: all satisfied",
			LabelSelector{
				{Key: "env", Operator: LabelOperatorEqual, Value: "prod"},
				{Key: "team", Operator: LabelOperatorExists},
				{Key: "debug", Operator: LabelOperatorNotExists},
			},
			map[string]string{"env": "prod", "team": "platform"},
			true,
		},
		{
			"all clauses ANDed: one fails",
			LabelSelector{
				{Key: "env", Operator: LabelOperatorEqual, Value: "prod"},
				{Key: "team", Operator: LabelOperatorExists},
			},
			map[string]string{"env": "prod"}, // team missing
			false,
		},
		{
			"unknown operator fails closed",
			LabelSelector{{Key: "x", Operator: LabelOperator("bogus"), Value: "y"}},
			map[string]string{"x": "y"},
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.selector.Matches(tc.labels))
		})
	}
}

// TestLabelSelector_Matches_RoundtripWithParser ensures Parse + Matches
// give the same answer as authoring the LabelSelector by hand. Mirrors the
// existing SQL roundtrip test.
func TestLabelSelector_Matches_RoundtripWithParser(t *testing.T) {
	parsed, err := ParseLabelSelector("env=prod,team,!debug")
	assert.NoError(t, err)

	assert.True(t, parsed.Matches(map[string]string{"env": "prod", "team": "platform"}))
	assert.False(t, parsed.Matches(map[string]string{"env": "prod"}))
	assert.False(t, parsed.Matches(map[string]string{"env": "prod", "team": "p", "debug": "1"}))
}
