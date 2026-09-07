import * as React from 'react';
import { CssBaseline, useMediaQuery } from '@mui/material';
import { PaletteMode, ThemeProvider } from '@mui/material/styles';
import { MarketplaceColorMode } from './marketplaceConfig';
import { createMarketplaceTheme } from './theme';

export const resolvePaletteMode = (
  configuredMode: MarketplaceColorMode,
  systemPrefersDark: boolean,
): PaletteMode => {
  if (configuredMode === 'system') {
    return systemPrefersDark ? 'dark' : 'light';
  }
  return configuredMode;
};

export default function MarketplaceThemeProvider({
  colorMode,
  children,
}: React.PropsWithChildren<{ colorMode: MarketplaceColorMode }>) {
  const systemPrefersDark = useMediaQuery('(prefers-color-scheme: dark)', { noSsr: true });
  const paletteMode = resolvePaletteMode(colorMode, systemPrefersDark);
  const theme = React.useMemo(() => createMarketplaceTheme(paletteMode), [paletteMode]);

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline enableColorScheme />
      {children}
    </ThemeProvider>
  );
}
