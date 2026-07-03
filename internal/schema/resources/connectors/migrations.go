package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Migrations defines the optional connector-authored hooks used when a
// connection moves between connector versions. Hooks are evaluated at runtime
// against the connector-level JavaScript library for the version being crossed.
type Migrations struct {
	Up   *MigrationHook `json:"up,omitempty" yaml:"up,omitempty"`
	Down *MigrationHook `json:"down,omitempty" yaml:"down,omitempty"`
}

func (m *Migrations) Clone() *Migrations {
	if m == nil {
		return nil
	}

	clone := *m
	if m.Up != nil {
		clone.Up = m.Up.Clone()
	}
	if m.Down != nil {
		clone.Down = m.Down.Clone()
	}
	return &clone
}

func (m *Migrations) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}
	if m.Up != nil {
		if err := m.Up.Validate(vc.PushField("up")); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if m.Down != nil {
		if err := m.Down.Validate(vc.PushField("down")); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result.ErrorOrNil()
}

// MigrationHook describes one JavaScript expression. The expression should
// return an object describing config, label, annotation, and notification
// patches.
type MigrationHook struct {
	Javascript string `json:"javascript" yaml:"javascript"`
}

func (h *MigrationHook) Clone() *MigrationHook {
	if h == nil {
		return nil
	}
	clone := *h
	return &clone
}

func (h *MigrationHook) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}
	if h.Javascript == "" {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "migration javascript is required"))
		return result.ErrorOrNil()
	}
	if err := apjs.ValidateExpressionSyntax(h.Javascript); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "invalid migration javascript expression: %v", err))
	}
	return result.ErrorOrNil()
}
