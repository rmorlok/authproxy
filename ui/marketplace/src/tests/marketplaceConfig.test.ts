import { describe, expect, it, vi } from 'vitest';
import {
  defaultMarketplaceColorMode,
  loadMarketplaceColorMode,
  marketplaceConfigUrl,
} from '../marketplaceConfig';

const responseWith = (body: unknown, ok = true) => ({
  ok,
  json: vi.fn().mockResolvedValue(body),
}) as unknown as Response;

describe('marketplace deployment config', () => {
  it('builds same-origin and separate-origin config URLs', () => {
    expect(marketplaceConfigUrl('')).toBe('/api/v1/marketplace/config');
    expect(marketplaceConfigUrl('https://public.example.com/')).toBe(
      'https://public.example.com/api/v1/marketplace/config',
    );
  });

  it.each(['light', 'dark', 'system'] as const)('loads the %s color mode', async (mode) => {
    const fetchConfig = vi.fn().mockResolvedValue(responseWith({ colorMode: mode }));

    await expect(loadMarketplaceColorMode('https://public.example.com/', fetchConfig)).resolves.toBe(mode);
    expect(fetchConfig).toHaveBeenCalledWith(
      'https://public.example.com/api/v1/marketplace/config',
      {
        credentials: 'include',
        headers: { Accept: 'application/json' },
      },
    );
  });

  it('uses the default for invalid responses and request failures', async () => {
    const invalidConfig = vi.fn().mockResolvedValue(responseWith({ colorMode: 'sepia' }));
    const failedResponse = vi.fn().mockResolvedValue(responseWith({}, false));
    const failedRequest = vi.fn().mockRejectedValue(new Error('offline'));

    await expect(loadMarketplaceColorMode('', invalidConfig)).resolves.toBe(defaultMarketplaceColorMode);
    await expect(loadMarketplaceColorMode('', failedResponse)).resolves.toBe(defaultMarketplaceColorMode);
    await expect(loadMarketplaceColorMode('', failedRequest)).resolves.toBe(defaultMarketplaceColorMode);
  });
});
