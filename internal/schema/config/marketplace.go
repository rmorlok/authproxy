package config

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

type MarketplaceColorMode string

const (
	MarketplaceColorModeLight  MarketplaceColorMode = "light"
	MarketplaceColorModeDark   MarketplaceColorMode = "dark"
	MarketplaceColorModeSystem MarketplaceColorMode = "system"
)

type Marketplace struct {
	BaseUrl   *StringValue          `json:"baseUrl,omitempty" yaml:"baseUrl,omitempty"`
	ColorMode *MarketplaceColorMode `json:"colorMode,omitempty" yaml:"colorMode,omitempty"`
}

// GetColorMode returns the palette mode used by the Marketplace UI. Following
// the browser's system setting is the default for deployments that do not pin
// a mode explicitly.
func (m *Marketplace) GetColorMode() MarketplaceColorMode {
	if m == nil || m.ColorMode == nil {
		return MarketplaceColorModeSystem
	}

	return *m.ColorMode
}

func (m *Marketplace) Validate(vc *common.ValidationContext) error {
	if m == nil || m.ColorMode == nil {
		return nil
	}

	result := &multierror.Error{}
	switch *m.ColorMode {
	case MarketplaceColorModeLight, MarketplaceColorModeDark, MarketplaceColorModeSystem:
		// Valid.
	default:
		result = multierror.Append(result, vc.NewErrorfForField(
			"colorMode",
			"must be one of %q, %q, or %q, got %q",
			MarketplaceColorModeLight,
			MarketplaceColorModeDark,
			MarketplaceColorModeSystem,
			*m.ColorMode,
		))
	}

	return result.ErrorOrNil()
}
