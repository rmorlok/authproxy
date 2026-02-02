package database

import (
	"fmt"
	"sort"
	"strings"

	sq "github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
)

type LabelOperator string

const (
	LabelOperatorEqual     LabelOperator = "="
	LabelOperatorNotEqual  LabelOperator = "!="
	LabelOperatorExists    LabelOperator = "exists"
	LabelOperatorNotExists LabelOperator = "!exists"
)

type LabelRequirement struct {
	Key      string
	Operator LabelOperator
	Value    string
}

type LabelSelector []LabelRequirement

// ParseLabelSelector parses a Kubernetes-style label selector string.
// Supported syntax:
// - key=value, key==value
// - key!=value
// - key (exists)
// - !key (does not exist)
func ParseLabelSelector(selector string) (LabelSelector, error) {
	if selector == "" {
		return nil, nil
	}

	var requirements LabelSelector
	parts := strings.Split(selector, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "!=") {
			opParts := strings.SplitN(part, "!=", 2)
			key := strings.TrimSpace(opParts[0])
			val := strings.TrimSpace(opParts[1])
			if err := ValidateLabelKey(key); err != nil {
				return nil, errors.Wrapf(err, "invalid label key in selector %q", part)
			}
			if err := ValidateLabelValue(val); err != nil {
				return nil, errors.Wrapf(err, "invalid label value in selector %q", part)
			}
			requirements = append(requirements, LabelRequirement{
				Key:      key,
				Operator: LabelOperatorNotEqual,
				Value:    val,
			})
		} else if strings.Contains(part, "==") {
			opParts := strings.SplitN(part, "==", 2)
			key := strings.TrimSpace(opParts[0])
			val := strings.TrimSpace(opParts[1])
			if err := ValidateLabelKey(key); err != nil {
				return nil, errors.Wrapf(err, "invalid label key in selector %q", part)
			}
			if err := ValidateLabelValue(val); err != nil {
				return nil, errors.Wrapf(err, "invalid label value in selector %q", part)
			}
			requirements = append(requirements, LabelRequirement{
				Key:      key,
				Operator: LabelOperatorEqual,
				Value:    val,
			})
		} else if strings.Contains(part, "=") {
			opParts := strings.SplitN(part, "=", 2)
			key := strings.TrimSpace(opParts[0])
			val := strings.TrimSpace(opParts[1])
			if err := ValidateLabelKey(key); err != nil {
				return nil, errors.Wrapf(err, "invalid label key in selector %q", part)
			}
			if err := ValidateLabelValue(val); err != nil {
				return nil, errors.Wrapf(err, "invalid label value in selector %q", part)
			}
			requirements = append(requirements, LabelRequirement{
				Key:      key,
				Operator: LabelOperatorEqual,
				Value:    val,
			})
		} else if strings.HasPrefix(part, "!") {
			key := strings.TrimSpace(part[1:])
			if err := ValidateLabelKey(key); err != nil {
				return nil, errors.Wrapf(err, "invalid label key in selector %q", part)
			}
			requirements = append(requirements, LabelRequirement{
				Key:      key,
				Operator: LabelOperatorNotExists,
			})
		} else {
			key := strings.TrimSpace(part)
			if err := ValidateLabelKey(key); err != nil {
				return nil, errors.Wrapf(err, "invalid label key in selector %q", part)
			}
			requirements = append(requirements, LabelRequirement{
				Key:      key,
				Operator: LabelOperatorExists,
			})
		}
	}

	return requirements, nil
}

func (s LabelSelector) String() string {
	var parts []string
	for _, r := range s {
		switch r.Operator {
		case LabelOperatorEqual:
			parts = append(parts, fmt.Sprintf("%s=%s", r.Key, r.Value))
		case LabelOperatorNotEqual:
			parts = append(parts, fmt.Sprintf("%s!=%s", r.Key, r.Value))
		case LabelOperatorExists:
			parts = append(parts, r.Key)
		case LabelOperatorNotExists:
			parts = append(parts, "!"+r.Key)
		}
	}
	return strings.Join(parts, ",")
}

func (s LabelSelector) ApplyToSqlBuilder(q sq.SelectBuilder, labelsColumn string) sq.SelectBuilder {
	for _, r := range s {
		switch r.Operator {
		case LabelOperatorEqual:
			q = q.Where(sq.Expr(fmt.Sprintf("json_extract(%s, '$.' || ?) = ?", labelsColumn), r.Key, r.Value))
		case LabelOperatorNotEqual:
			// For inequality, we need to handle the case where the key doesn't exist
			q = q.Where(sq.Expr(fmt.Sprintf("(json_extract(%s, '$.' || ?) IS NULL OR json_extract(%s, '$.' || ?) != ?)", labelsColumn, labelsColumn), r.Key, r.Key, r.Value))
		case LabelOperatorExists:
			q = q.Where(sq.Expr(fmt.Sprintf("json_extract(%s, '$.' || ?) IS NOT NULL", labelsColumn), r.Key))
		case LabelOperatorNotExists:
			q = q.Where(sq.Expr(fmt.Sprintf("json_extract(%s, '$.' || ?) IS NULL", labelsColumn), r.Key))
		}
	}
	return q
}

// BuildLabelSelectorFromMap creates a label selector string from key-value pairs.
// Keys are sorted for deterministic output.
// Example: {"type": "salesforce", "env": "prod"} -> "env=prod,type=salesforce"
func BuildLabelSelectorFromMap(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}
	return strings.Join(parts, ",")
}
