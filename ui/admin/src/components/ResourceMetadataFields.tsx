import React, {useMemo} from 'react';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';

const monoFontFamily = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Roboto Mono", monospace';

export function ResourceNamespace({namespace}: {namespace: string}) {
  return (
    <Box sx={{flexShrink: 0}}>
      <Typography variant="subtitle2" color="text.secondary">Namespace</Typography>
      <Typography
        variant="body1"
        component="code"
        sx={{
          display: 'inline-block',
          mt: 0.5,
          fontFamily: monoFontFamily,
          fontSize: '0.9rem',
          wordBreak: 'break-all',
        }}
      >
        {namespace}
      </Typography>
    </Box>
  );
}

export function ResourceLabels({labels}: {labels?: Record<string, string>}) {
  return (
    <Box>
      <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
      <ResourceLabelChips labels={labels}/>
    </Box>
  );
}

export function ResourceLabelChips({labels}: {labels?: Record<string, string>}) {
  const entries = useMemo(
    () => Object.entries(labels || {}).sort(([left], [right]) => left.localeCompare(right)),
    [labels],
  );

  if (entries.length === 0) {
    return <Typography variant="body2" color="text.secondary" sx={{mt: 0.5}}>No labels</Typography>;
  }

  return (
    <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{mt: 0.5, rowGap: 0.5}}>
      {entries.map(([key, value]) => (
        <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined"/>
      ))}
    </Stack>
  );
}
