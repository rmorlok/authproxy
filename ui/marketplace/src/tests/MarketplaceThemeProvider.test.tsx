import * as React from 'react';
import { act, render, screen } from '@testing-library/react';
import { useTheme } from '@mui/material/styles';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MarketplaceThemeProvider, { resolvePaletteMode } from '../MarketplaceThemeProvider';

const ThemeMode = () => {
  const theme = useTheme();
  return <span>{theme.palette.mode}</span>;
};

afterEach(() => {
  vi.restoreAllMocks();
});

describe('MarketplaceThemeProvider', () => {
  it('resolves forced and system palette modes', () => {
    expect(resolvePaletteMode('light', true)).toBe('light');
    expect(resolvePaletteMode('dark', false)).toBe('dark');
    expect(resolvePaletteMode('system', false)).toBe('light');
    expect(resolvePaletteMode('system', true)).toBe('dark');
  });

  it.each(['light', 'dark'] as const)('uses forced %s mode', (colorMode) => {
    render(
      <MarketplaceThemeProvider colorMode={colorMode}>
        <ThemeMode />
      </MarketplaceThemeProvider>,
    );

    expect(screen.getByText(colorMode)).toBeInTheDocument();
  });

  it('reacts when the system color scheme changes', () => {
    let listener: ((event: MediaQueryListEvent) => void) | undefined;
    const mediaQueryList = {
      matches: false,
      media: '(prefers-color-scheme: dark)',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: (_event: string, callback: EventListenerOrEventListenerObject) => {
        listener = callback as (event: MediaQueryListEvent) => void;
      },
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    } as unknown as MediaQueryList;
    vi.spyOn(window, 'matchMedia').mockReturnValue(mediaQueryList);

    render(
      <MarketplaceThemeProvider colorMode="system">
        <ThemeMode />
      </MarketplaceThemeProvider>,
    );
    expect(screen.getByText('light')).toBeInTheDocument();

    Object.defineProperty(mediaQueryList, 'matches', { value: true });
    act(() => listener?.({ matches: true } as MediaQueryListEvent));
    expect(screen.getByText('dark')).toBeInTheDocument();
  });
});
