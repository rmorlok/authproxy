package config

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/require"
)

func TestMarketplaceGetColorMode(t *testing.T) {
	t.Run("defaults to system when marketplace is nil", func(t *testing.T) {
		var marketplace *Marketplace
		require.Equal(t, MarketplaceColorModeSystem, marketplace.GetColorMode())
	})

	t.Run("defaults to system when color mode is omitted", func(t *testing.T) {
		require.Equal(t, MarketplaceColorModeSystem, (&Marketplace{}).GetColorMode())
	})

	t.Run("returns configured color mode", func(t *testing.T) {
		mode := MarketplaceColorModeDark
		require.Equal(t, MarketplaceColorModeDark, (&Marketplace{ColorMode: &mode}).GetColorMode())
	})
}

func TestMarketplaceValidate(t *testing.T) {
	vc := &common.ValidationContext{Path: "$.marketplace"}

	for _, mode := range []MarketplaceColorMode{
		MarketplaceColorModeLight,
		MarketplaceColorModeDark,
		MarketplaceColorModeSystem,
	} {
		t.Run(string(mode), func(t *testing.T) {
			require.NoError(t, (&Marketplace{ColorMode: &mode}).Validate(vc))
		})
	}

	invalid := MarketplaceColorMode("sepia")
	require.ErrorContains(t, (&Marketplace{ColorMode: &invalid}).Validate(vc), "must be one of")
}
