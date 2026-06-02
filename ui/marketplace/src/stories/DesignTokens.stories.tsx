import * as React from 'react';
import { Meta, StoryObj } from '@storybook/react';
import {
  Box,
  Button,
  Chip,
  Paper,
  Stack,
  ThemeProvider,
  Typography,
} from '@mui/material';
import theme, { marketplaceTokens } from '../theme';

function TokenSwatch({
  label,
  color,
}: {
  label: string;
  color: 'success' | 'warning' | 'error' | 'default';
}) {
  return (
    <Stack direction="row" spacing={1} alignItems="center">
      <Chip label={label} color={color} size="small" variant={color === 'default' ? 'outlined' : 'filled'} />
      <Typography variant="body2" color="text.secondary">
        {color}
      </Typography>
    </Stack>
  );
}

function DesignTokens() {
  return (
    <ThemeProvider theme={theme}>
      <Box sx={{ p: 4, maxWidth: 960 }}>
        <Typography variant="h4" gutterBottom>
          Marketplace Design Tokens
        </Typography>
        <Typography variant="body1" color="text.secondary" sx={{ mb: 4 }}>
          Shared marketplace spacing, radii, status, and action conventions.
        </Typography>

        <Stack spacing={3}>
          <Paper
            variant="outlined"
            sx={{
              p: marketplaceTokens.spacing.panelPadding,
              borderRadius: marketplaceTokens.radius.panel,
            }}
          >
            <Typography variant="h6" gutterBottom>
              Status treatments
            </Typography>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
              <TokenSwatch label="configured" color={marketplaceTokens.status.healthy} />
              <TokenSwatch label="Needs attention" color={marketplaceTokens.status.attention} />
              <TokenSwatch label="setup" color={marketplaceTokens.status.setup} />
              <TokenSwatch label="disabled" color={marketplaceTokens.status.disabled} />
            </Stack>
          </Paper>

          <Paper
            variant="outlined"
            sx={{
              p: marketplaceTokens.spacing.panelPadding,
              borderRadius: marketplaceTokens.radius.card,
              boxShadow: marketplaceTokens.card.shadow,
            }}
          >
            <Typography variant="h6" gutterBottom>
              Action hierarchy
            </Typography>
            <Stack direction="row" spacing={1} flexWrap="wrap">
              <Button variant="contained">Primary action</Button>
              <Button variant="text">Secondary action</Button>
              <Button color="error" variant="text">Destructive action</Button>
            </Stack>
          </Paper>

          <Paper
            variant="outlined"
            sx={{
              p: marketplaceTokens.spacing.panelPadding,
              borderRadius: marketplaceTokens.radius.panel,
            }}
          >
            <Typography variant="h6" gutterBottom>
              Layout scale
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Page Y: {marketplaceTokens.spacing.pageY}, grid gap: {marketplaceTokens.spacing.gridGap},
              panel radius: {marketplaceTokens.radius.panel}, card radius: {marketplaceTokens.radius.card}
            </Typography>
          </Paper>
        </Stack>
      </Box>
    </ThemeProvider>
  );
}

const meta: Meta<typeof DesignTokens> = {
  title: 'Pages/Marketplace/Design Tokens',
  component: DesignTokens,
  parameters: {
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof DesignTokens>;

export const Reference: Story = {};
