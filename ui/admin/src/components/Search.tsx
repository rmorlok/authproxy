import * as React from 'react';
import FormControl from '@mui/material/FormControl';
import InputAdornment from '@mui/material/InputAdornment';
import OutlinedInput from '@mui/material/OutlinedInput';
import SearchRoundedIcon from '@mui/icons-material/SearchRounded';
import Typography from '@mui/material/Typography';
import {useCommandPalette} from '../search/CommandPalette';

export default function Search() {
  const commandPalette = useCommandPalette();

  return (
    <FormControl sx={{ width: { xs: '100%', md: '25ch' } }} variant="outlined">
      <OutlinedInput
        size="small"
        id="admin-command-palette-trigger"
        placeholder="Search resources…"
        readOnly
        onClick={() => commandPalette.open()}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            commandPalette.open();
          }
        }}
        sx={{ flexGrow: 1, cursor: 'pointer' }}
        startAdornment={
          <InputAdornment position="start" sx={{ color: 'text.primary' }}>
            <SearchRoundedIcon fontSize="small" />
          </InputAdornment>
        }
        endAdornment={
          <InputAdornment position="end">
            <Typography
              component="kbd"
              variant="caption"
              sx={{color: 'text.secondary', fontFamily: 'monospace'}}
            >
              ⌘K
            </Typography>
          </InputAdornment>
        }
        inputProps={{
          'aria-label': 'Open resource search',
        }}
      />
    </FormControl>
  );
}
