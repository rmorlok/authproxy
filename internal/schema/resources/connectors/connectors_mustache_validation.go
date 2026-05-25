package connectors

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/aptmpl"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// cfgPrefix is the mustache namespace for fields collected during setup flow forms
// and stored on the connection as Configuration. References like {{cfg.tenant}} resolve
// from this map at render time.
const cfgPrefix = "cfg."

// validateMustacheReferences orchestrates per-field mustache cross-validation across
// the connector. It computes the cfg.X field sets available at each lifecycle point
// and delegates to the validators that live next to the templated fields themselves
// (AuthOAuth2.ValidateMustacheReferences, DataSourceProxyRequest.ValidateMustacheReferences).
//
// {{labels.X}} and {{annotations.X}} references are not validated since those are set
// freely on each connection and are not constrained by the connector definition.
func (c *Connector) validateMustacheReferences(vc *common.ValidationContext) error {
	if c == nil || c.Auth == nil {
		return nil
	}

	mctx := NewMustacheValidationContext(c.SetupFlow)
	result := &multierror.Error{}

	if v, ok := c.Auth.Inner().(MustacheValidator); ok {
		if err := v.ValidateMustacheReferences(vc.PushField("auth"), mctx); err != nil {
			result = multierror.Append(result, err)
		}
	}

	if c.SetupFlow != nil && c.SetupFlow.Configure != nil {
		for stepIdx, step := range c.SetupFlow.Configure.Steps {
			if len(step.DataSources) == 0 {
				continue
			}
			stepVc := vc.PushField("setup_flow").PushField("configure").PushField("steps").PushIndex(stepIdx).PushField("data_sources")
			for name, ds := range step.DataSources {
				if err := ds.ProxyRequest.ValidateMustacheReferences(stepVc.PushField(name).PushField("proxy_request"), mctx, stepIdx); err != nil {
					result = multierror.Append(result, err)
				}
			}
		}
	}

	return result.ErrorOrNil()
}

// checkMustacheTemplate parses the template, extracts cfg references, and reports any
// missing fields against the provided available-set with the given scope description.
// Used by the per-struct ValidateMustacheReferences methods.
func checkMustacheTemplate(vc *common.ValidationContext, template string, available map[string]bool, scope string, result *multierror.Error) {
	if template == "" {
		return
	}
	vars, err := aptmpl.ExtractVariables(template)
	if err != nil {
		*result = *multierror.Append(result, vc.NewErrorf("invalid mustache template: %s", err.Error()))
		return
	}
	for _, v := range vars {
		if !strings.HasPrefix(v, cfgPrefix) {
			continue
		}
		field := strings.TrimPrefix(v, cfgPrefix)
		// Only validate the top-level field; nested paths like cfg.foo.bar still require
		// "foo" to be declared by the setup flow.
		root := field
		if idx := strings.IndexByte(field, '.'); idx >= 0 {
			root = field[:idx]
		}
		if root == "" {
			*result = *multierror.Append(result, vc.NewErrorf("mustache reference %q has empty cfg field name", "{{"+v+"}}"))
			continue
		}
		if !available[root] {
			*result = *multierror.Append(result, vc.NewErrorf(
				"mustache reference %q has no matching field in %s of the setup flow",
				"{{"+v+"}}", scope,
			))
		}
	}
}

func configureScopeLabel(stepIdx int) string {
	if stepIdx == 0 {
		return "preconnect"
	}
	return fmt.Sprintf("preconnect or configure steps before index %d", stepIdx)
}
