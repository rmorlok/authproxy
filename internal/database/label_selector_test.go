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
