import { createTheme } from '@mui/material/styles';
import { red } from '@mui/material/colors';

export const marketplaceTokens = {
  radius: {
    card: 1,
    panel: 2,
    control: 1,
  },
  spacing: {
    pageY: 4,
    headerGap: 2,
    gridGap: 4,
    sectionGap: 4,
    panelPadding: { xs: 3, sm: 4 },
    cardActionGap: 0.5,
  },
  card: {
    borderColor: 'divider',
    surface: 'background.paper',
    shadow: 1,
    attentionShadow: 4,
    mediaHeight: 140,
  },
  markdown: {
    bodyFontSize: '0.875rem',
    codeFontSize: '0.8rem',
    codePadding: '2px 4px',
  },
  status: {
    healthy: 'success',
    attention: 'warning',
    disabled: 'error',
    setup: 'warning',
    neutral: 'default',
  },
} as const;

// A custom theme for this app. Keep brand colors in MUI's palette and marketplace
// layout/status conventions in marketplaceTokens so a host app can replace either layer.
const theme = createTheme({
  shape: {
    borderRadius: 8,
  },
  palette: {
    primary: {
      main: '#556cd6',
    },
    secondary: {
      main: '#19857b',
    },
    error: {
      main: red.A400,
    },
  },
  components: {
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: 8,
        },
      },
    },
    MuiButton: {
      defaultProps: {
        disableElevation: true,
      },
      styleOverrides: {
        root: {
          textTransform: 'none',
          fontWeight: 600,
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          borderRadius: 8,
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          fontWeight: 600,
        },
      },
    },
  },
});

export default theme;
