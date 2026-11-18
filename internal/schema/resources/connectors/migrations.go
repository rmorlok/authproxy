package connectors

import (
	"context"
	"errors"

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

// Validate checks each hook independently under the caller's
// validation context and the combined library-plus-hook source allowance.
// Neither the hooks nor the supplied library are executed by this validation.
// A nil library is supported. Caller cancellation stops validation immediately
// and is returned directly rather than collected as a hook definition error.
func (m *Migrations) Validate(
	vc *common.ValidationContext,
	library *apjs.Library,
) error {
	ctx := vc.Context()
	if err := ctx.Err(); err != nil {
		return err
	}
	result := &multierror.Error{}

	if m.Up != nil {
		if err := m.Up.Validate(vc.PushField("up"), library); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			result = multierror.Append(result, err)
		}
	}

	if m.Down != nil {
		if err := m.Down.Validate(vc.PushField("down"), library); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			result = multierror.Append(result, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
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

// Validate bounds source compilation using the same allowance as
// migration execution, including the library's original source bytes. Errors
// retain the javascript field path and safe syntax positions without authored
// parser messages. Library initialization remains the connector's responsibility.
// A nil library is supported. Caller cancellation is an operational failure,
// returned directly instead of being labeled as invalid migration JavaScript.
func (h *MigrationHook) Validate(
	vc *common.ValidationContext,
	library *apjs.Library,
) error {
	ctx := vc.Context()
	if err := ctx.Err(); err != nil {
		return err
	}
	result := &multierror.Error{}

	if h.Javascript == "" {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "migration javascript is required"))
		return result.ErrorOrNil()
	}

	err := library.ValidateExpressionSyntaxWithContext(
		ctx,
		h.Javascript,
		apjs.ConnectorMigrationLimits(),
	)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if err != nil {
		result = multierror.Append(result, vc.NewErrorfForField("javascript", "invalid migration javascript expression: %v", err))
	}

	return result.ErrorOrNil()
}
