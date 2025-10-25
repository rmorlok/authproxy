package connectors

import "github.com/rmorlok/authproxy/config/common"

// Probe is a mechanism for a connection to validate the state of the world. It can be used to establish if
// a connection still has valid credentials or if the 3rd party system has certain capabilities.
type Probe struct {
	// Id is the unique id the probe. This must be unique within the connector definition.
	Id string `json:"id" yaml:"id"`

	// Period is how frequently the probe should be run
	Period common.HumanDuration `json:"period" yaml:"period"`

	// ProxyHttp is an defines the probe as using an Authenticated HTTP proxy call as the probe.
	ProxyHttp *ProbeHttp `json:"proxy_http,omitempty" yaml:"proxy_http,omitempty"`

	// Http is an defines the probe as using a raw HTTP request as the probe.
	Http *ProbeHttp `json:"http,omitempty" yaml:"http,omitempty"`
}

func (p *Probe) Validate(vc *common.ValidationContext) error {
	typeCount := 0
	if p.ProxyHttp != nil {
		typeCount++
	}
	if p.Http != nil {
		typeCount++
	}
	if typeCount != 1 {
		return vc.NewErrorf("exactly one of proxy_http or http must be defined")
	}

	return nil
}
