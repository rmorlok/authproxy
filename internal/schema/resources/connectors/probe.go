package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/robfig/cron/v3"
)

// Default thresholds for the probe-driven health-check signal. Probes that
// omit FailureThreshold / RecoveryThreshold inherit these. Exported so the
// runtime (which actually counts probe outcomes and flips connection health)
// can read the same constants the YAML layer documents.
const (
	// DefaultProbeFailureThreshold is the number of consecutive probe failures
	// required to flip a connection's health_state to unhealthy when the probe
	// definition omits its own failure_threshold.
	DefaultProbeFailureThreshold = 3

	// DefaultProbeRecoveryThreshold is the number of consecutive probe successes
	// required to flip a connection's health_state back to healthy after a
	// previous failure threshold-cross, when the probe definition omits its own
	// recovery_threshold.
	DefaultProbeRecoveryThreshold = 1
)

// Probe is a mechanism for a connection to validate the state of the world. It can be used to establish if
// a connection still has valid credentials or if the 3rd party system has certain capabilities.
type Probe struct {
	// Id is the unique id the probe. This must be unique within the connector definition.
	Id string `json:"id" yaml:"id"`

	// Period is how frequently the probe should be run. This can be expressed as an alternative to a cron for
	// probes that should be run periodically. This is exclusive with the cron field.
	Period *common.HumanDuration `json:"period,omitempty" yaml:"period,omitempty"`

	// Cron is the cron expression for a cadence to run the probe. This is exclusive with the period field.
	Cron *string `json:"cron,omitempty" yaml:"cron,omitempty"`

	// ProxyHttp defines the probe as using an Authenticated HTTP proxy call as the probe.
	ProxyHttp *ProbeHttp `json:"proxy_http,omitempty" yaml:"proxy_http,omitempty"`

	// Http defines the probe as using a raw HTTP request as the probe.
	Http *ProbeHttp `json:"http,omitempty" yaml:"http,omitempty"`

	// If optionally disables this probe for a connection. Runtime evaluates
	// the configured condition server-side with cfg, labels, and annotations
	// variables in scope.
	If *common.Predicate `json:"if,omitempty" yaml:"if,omitempty"`

	// FailureThreshold is the number of consecutive failures that must occur
	// before the connection's health_state flips to unhealthy. Defaults to
	// DefaultProbeFailureThreshold when omitted. Must be ≥ 1 when set.
	FailureThreshold *int `json:"failure_threshold,omitempty" yaml:"failure_threshold,omitempty"`

	// RecoveryThreshold is the number of consecutive successes that must occur,
	// while the connection is unhealthy, before health_state flips back to
	// healthy. Defaults to DefaultProbeRecoveryThreshold when omitted. Must be
	// ≥ 1 when set.
	RecoveryThreshold *int `json:"recovery_threshold,omitempty" yaml:"recovery_threshold,omitempty"`
}

// EffectiveFailureThreshold returns the configured failure threshold, falling
// back to DefaultProbeFailureThreshold when the field is unset.
func (p *Probe) EffectiveFailureThreshold() int {
	if p == nil || p.FailureThreshold == nil {
		return DefaultProbeFailureThreshold
	}
	return *p.FailureThreshold
}

// EffectiveRecoveryThreshold returns the configured recovery threshold, falling
// back to DefaultProbeRecoveryThreshold when the field is unset.
func (p *Probe) EffectiveRecoveryThreshold() int {
	if p == nil || p.RecoveryThreshold == nil {
		return DefaultProbeRecoveryThreshold
	}
	return *p.RecoveryThreshold
}

func (p *Probe) Validate(vc *common.ValidationContext) error {
	return p.ValidateWithJavascript(vc, nil)
}

func (p *Probe) ValidateWithJavascript(vc *common.ValidationContext, library *apjs.Library) error {
	result := &multierror.Error{}

	typeCount := 0
	if p.ProxyHttp != nil {
		typeCount++
	}
	if p.Http != nil {
		typeCount++
	}

	if typeCount != 1 {
		result = multierror.Append(result, vc.NewErrorf("exactly one of proxy_http or http must be defined"))
	}

	if err := p.If.Validate(vc.PushField("if"), connectorPredicateValidationContext(library)); err != nil {
		result = multierror.Append(result, err)
	}

	if p.Period != nil && p.Cron != nil {
		result = multierror.Append(result, vc.NewErrorf("either period or cron may be defined"))
	}

	if p.Cron != nil {
		_, err := cron.ParseStandard(*p.Cron)
		if err != nil {
			result = multierror.Append(result, vc.PushField("cron").NewErrorf("cron expression is invalid: %v", err))
		}
	}

	if p.FailureThreshold != nil && *p.FailureThreshold < 1 {
		result = multierror.Append(result, vc.PushField("failure_threshold").NewErrorf("must be at least 1, got %d", *p.FailureThreshold))
	}

	if p.RecoveryThreshold != nil && *p.RecoveryThreshold < 1 {
		result = multierror.Append(result, vc.PushField("recovery_threshold").NewErrorf("must be at least 1, got %d", *p.RecoveryThreshold))
	}

	return result.ErrorOrNil()
}
