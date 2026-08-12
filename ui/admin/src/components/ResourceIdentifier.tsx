import React, {useState} from 'react';
import Box from '@mui/material/Box';
import IconButton from '@mui/material/IconButton';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';

const monoFontFamily = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Roboto Mono", monospace';

export default function ResourceIdentifier({
  label = 'ID',
  value,
  copyLabel,
}: {
  label?: string;
  value: string;
  copyLabel: string;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch (_err) {
      // Clipboard access can be unavailable in an insecure browser context.
    }
  };

  return (
    <Box>
      <Typography variant="subtitle2" color="text.secondary">{label}</Typography>
      <Stack direction="row" spacing={1} alignItems="center" sx={{mt: 0.5}}>
        <Typography
          variant="body1"
          component="code"
          sx={{
            wordBreak: 'break-all',
            fontFamily: monoFontFamily,
            bgcolor: 'action.hover',
            px: 1,
            py: 0.5,
            borderRadius: 0.5,
            fontSize: '0.9rem',
            letterSpacing: '0.02em',
          }}
        >
          {value}
        </Typography>
        <Tooltip title={copied ? 'Copied!' : 'Copy'} placement="top">
          <IconButton size="small" aria-label={copyLabel} onClick={handleCopy}>
            <ContentCopyIcon fontSize="inherit" />
          </IconButton>
        </Tooltip>
      </Stack>
    </Box>
  );
}
