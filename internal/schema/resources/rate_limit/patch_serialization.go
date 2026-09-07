package rate_limit

import (
	"encoding/json"

	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

type rateLimitSpecPatchWire struct {
	Scope     *RateLimitScope `json:"scope,omitempty" yaml:"scope,omitempty"`
	Mode      *Mode           `json:"mode,omitempty" yaml:"mode,omitempty"`
	Selector  *Selector       `json:"selector,omitempty" yaml:"selector,omitempty"`
	Bucket    *Bucket         `json:"bucket,omitempty" yaml:"bucket,omitempty"`
	Algorithm *Algorithm      `json:"algorithm,omitempty" yaml:"algorithm,omitempty"`
}

func (p RateLimitSpecPatch) MarshalJSON() ([]byte, error) {
	value := map[string]any{}
	if p.scopePresent || p.Scope != nil {
		value["scope"] = p.Scope
	}
	if p.modePresent || p.Mode != nil {
		value["mode"] = p.Mode
	}
	if p.selectorPresent || p.Selector != nil {
		value["selector"] = p.Selector
	}
	if p.bucketPresent || p.Bucket != nil {
		value["bucket"] = p.Bucket
	}
	if p.algorithmPresent || p.Algorithm != nil {
		value["algorithm"] = p.Algorithm
	}
	return json.Marshal(value)
}

func (p *RateLimitSpecPatch) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var wire rateLimitSpecPatchWire
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}
	p.assign(wire)
	_, p.scopePresent = fields["scope"]
	_, p.modePresent = fields["mode"]
	_, p.selectorPresent = fields["selector"]
	_, p.bucketPresent = fields["bucket"]
	_, p.algorithmPresent = fields["algorithm"]
	return nil
}

func (p RateLimitSpecPatch) MarshalYAML() (any, error) {
	value := map[string]any{}
	if p.scopePresent || p.Scope != nil {
		value["scope"] = p.Scope
	}
	if p.modePresent || p.Mode != nil {
		value["mode"] = p.Mode
	}
	if p.selectorPresent || p.Selector != nil {
		value["selector"] = p.Selector
	}
	if p.bucketPresent || p.Bucket != nil {
		value["bucket"] = p.Bucket
	}
	if p.algorithmPresent || p.Algorithm != nil {
		value["algorithm"] = p.Algorithm
	}
	return value, nil
}

func (p *RateLimitSpecPatch) UnmarshalYAML(value *yaml.Node) error {
	var wire rateLimitSpecPatchWire
	if err := util.DecodeYAMLNodeStrict(value, &wire); err != nil {
		return err
	}
	p.assign(wire)
	node := value
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			switch node.Content[i].Value {
			case "scope":
				p.scopePresent = true
			case "mode":
				p.modePresent = true
			case "selector":
				p.selectorPresent = true
			case "bucket":
				p.bucketPresent = true
			case "algorithm":
				p.algorithmPresent = true
			}
		}
	}
	return nil
}

func (p *RateLimitSpecPatch) assign(w rateLimitSpecPatchWire) {
	*p = RateLimitSpecPatch{Scope: w.Scope, Mode: w.Mode, Selector: w.Selector, Bucket: w.Bucket, Algorithm: w.Algorithm}
}
