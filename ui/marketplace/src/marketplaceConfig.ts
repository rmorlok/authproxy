export type MarketplaceColorMode = 'light' | 'dark' | 'system';

export const defaultMarketplaceColorMode: MarketplaceColorMode = 'system';

const isMarketplaceColorMode = (value: unknown): value is MarketplaceColorMode => (
  value === 'light' || value === 'dark' || value === 'system'
);

export const marketplaceConfigUrl = (publicBaseUrl: string): string => (
  `${publicBaseUrl.replace(/\/$/, '')}/api/v1/marketplace/config`
);

export async function loadMarketplaceColorMode(
  publicBaseUrl: string = import.meta.env.VITE_PUBLIC_BASE_URL ?? '',
  fetchConfig: typeof fetch = fetch,
): Promise<MarketplaceColorMode> {
  try {
    const response = await fetchConfig(marketplaceConfigUrl(publicBaseUrl), {
      credentials: 'include',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      return defaultMarketplaceColorMode;
    }

    const config: unknown = await response.json();
    if (
      typeof config === 'object'
      && config !== null
      && 'colorMode' in config
      && isMarketplaceColorMode(config.colorMode)
    ) {
      return config.colorMode;
    }
  } catch (_error) {
    // Keep the Marketplace usable with the local default when the runtime
    // configuration endpoint is unavailable (for example, during Storybook).
  }

  return defaultMarketplaceColorMode;
}
