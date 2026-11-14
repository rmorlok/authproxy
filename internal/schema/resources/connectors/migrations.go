package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Migrations defines the optional connector-authored hooks used when a
// connection moves between connector generations. Hooks are evaluated at runtime
// against the connector-level JavaScript library for the generation being crossed.
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

// Validate checks hook syntax without a connector library. It does not execute
// either hook; runtime configuration and helper calls need not be available.
func (m *Migrations) Validate(vc *common.ValidationContext) error {
	return m.ValidateWithJavascript(vc, nil)
}

// ValidateWithJavascript checks each hook independently under the caller's
// validation context and the combined library-plus-hook source allowance.
// Neither the hooks nor the supplied library are executed by this validation.
func (m *Migrations) ValidateWithJavascript(vc *common.ValidationContext, library *apjs.Library) error {
	result := &multierror.Error{}
	if m.Up != nil {
		if err := m.Up.ValidateWithJavascript(vc.PushField("up"), library); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if m.Down != nil {
		if err := m.Down.ValidateWithJavascript(vc.PushField("down"), library); err != nil {
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

// Validate checks required hook syntax with no connector library. Runtime
// behavior and the returned patch are checked when the migration is executed.
func (h *MigrationHook) Validate(vc *common.ValidationContext) error {
	return h.ValidateWithJavascript(vc, nil)
}

// ValidateWithJavascript bounds source compilation using the same allowance as
// migration execution, including the library's original source bytes. Errors
// retain the javascript field path and safe syntax positions without authored
// parser messages. Library initialization remains the connector's responsibility.
func (h *MigrationHook) ValidateWithJavascript(vc *common.ValidationContext, library *apjs.Library) error {
	result := &multierror.Error{}
	if h.Javascript == "" {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "migration javascript is required"))
		return result.ErrorOrNil()
	}
	if err := library.ValidateExpressionSyntaxWithContext(vc.Context(), h.Javascript, apjs.ConnectorMigrationLimits()); err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "invalid migration javascript expression: %v", err))
	}
	return result.ErrorOrNil()
}
