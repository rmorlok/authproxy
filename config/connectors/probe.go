package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/config/common"
	"github.com/robfig/cron/v3"
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
}

func (p *Probe) Validate(vc *common.ValidationContext) error {
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

	if p.Period != nil && p.Cron != nil {
		result = multierror.Append(result, vc.NewErrorf("either period or cron may be defined"))
	}

	if p.Cron != nil {
		_, err := cron.ParseStandard(*p.Cron)
		if err != nil {
			result = multierror.Append(result, vc.PushField("cron").NewErrorf("cron expression is invalid: %v", err))
		}
	}

	return result.ErrorOrNil()
}
