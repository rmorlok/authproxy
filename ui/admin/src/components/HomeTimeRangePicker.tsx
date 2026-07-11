import * as React from 'react';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Divider from '@mui/material/Divider';
import InputAdornment from '@mui/material/InputAdornment';
import List from '@mui/material/List';
import ListItemButton from '@mui/material/ListItemButton';
import Popover from '@mui/material/Popover';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import AccessTimeRoundedIcon from '@mui/icons-material/AccessTimeRounded';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import ContentPasteIcon from '@mui/icons-material/ContentPaste';
import SearchRoundedIcon from '@mui/icons-material/SearchRounded';
import {
    browserTimeZoneLabel,
    describeDashboardTimeRange,
    parseGrafanaTimeRange,
    QUICK_DASHBOARD_TIME_RANGES,
    rangesEqual,
    serializeGrafanaTimeRange,
    timeRangeValidationError,
} from '../metrics/timeRange';
import type {DashboardTimeRange} from '../metrics/timeRange';

interface HomeTimeRangePickerProps {
    value: DashboardTimeRange;
    onApply: (range: DashboardTimeRange) => void;
}

export default function HomeTimeRangePicker({value, onApply}: HomeTimeRangePickerProps) {
    const [anchorEl, setAnchorEl] = React.useState<HTMLElement | null>(null);
    const [draftFrom, setDraftFrom] = React.useState(value.from);
    const [draftTo, setDraftTo] = React.useState(value.to);
    const [search, setSearch] = React.useState('');
    const [error, setError] = React.useState<string | null>(null);
    const [status, setStatus] = React.useState<string | null>(null);
    const open = Boolean(anchorEl);
    const draftRange = React.useMemo(
        () => ({from: draftFrom.trim(), to: draftTo.trim()}),
        [draftFrom, draftTo],
    );
    const timeZoneLabel = React.useMemo(() => browserTimeZoneLabel(), []);
    const selectedQuickRange = QUICK_DASHBOARD_TIME_RANGES.find((item) => rangesEqual(item.range, draftRange));
    const filteredQuickRanges = QUICK_DASHBOARD_TIME_RANGES.filter((item) => (
        item.label.toLowerCase().includes(search.trim().toLowerCase())
    ));

    React.useEffect(() => {
        if (open) {
            setDraftFrom(value.from);
            setDraftTo(value.to);
            setSearch('');
            setError(null);
            setStatus(null);
        }
    }, [open, value]);

    const close = () => setAnchorEl(null);

    const applyRange = (nextRange: DashboardTimeRange, closeAfterApply = true) => {
        const validationError = timeRangeValidationError(nextRange);
        if (validationError) {
            setError(validationError);
            setStatus(null);
            return;
        }

        onApply(nextRange);
        setError(null);
        setStatus(null);
        if (closeAfterApply) {
            close();
        }
    };

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(serializeGrafanaTimeRange(draftRange));
            setStatus('Copied');
            setError(null);
        } catch (_err) {
            setStatus(null);
            setError('Clipboard copy failed');
        }
    };

    const handlePaste = async () => {
        try {
            const clipboardValue = await navigator.clipboard.readText();
            const parsed = parseGrafanaTimeRange(clipboardValue);
            const validationError = timeRangeValidationError(parsed);
            if (validationError) {
                setStatus(null);
                setError(validationError);
                return;
            }
            setDraftFrom(parsed.from);
            setDraftTo(parsed.to);
            setStatus('Pasted');
            setError(null);
        } catch (err) {
            setStatus(null);
            setError(err instanceof Error ? err.message : 'Clipboard paste failed');
        }
    };

    const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
        if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
            event.preventDefault();
            applyRange(draftRange);
        }
    };

    return (
        <>
            <Button
                variant="outlined"
                size="small"
                startIcon={<AccessTimeRoundedIcon fontSize="small" />}
                onClick={(event) => setAnchorEl(event.currentTarget)}
                aria-haspopup="dialog"
                aria-expanded={open ? 'true' : undefined}
                sx={{
                    minWidth: {xs: '100%', sm: 176},
                    justifyContent: 'flex-start',
                    whiteSpace: 'nowrap',
                }}
            >
                {describeDashboardTimeRange(value)}
            </Button>
            <Popover
                open={open}
                anchorEl={anchorEl}
                onClose={close}
                anchorOrigin={{vertical: 'bottom', horizontal: 'right'}}
                transformOrigin={{vertical: 'top', horizontal: 'right'}}
                PaperProps={{
                    sx: {
                        width: {xs: 'calc(100vw - 32px)', sm: 780},
                        maxWidth: 'calc(100vw - 32px)',
                        mt: 1,
                        overflow: 'hidden',
                    },
                }}
            >
                <Stack
                    direction={{xs: 'column', md: 'row'}}
                    divider={<Divider orientation="vertical" flexItem />}
                    onKeyDown={handleKeyDown}
                >
                    <Stack spacing={2} sx={{p: 2.5, flex: '1 1 56%', minWidth: 0}}>
                        <Typography variant="h6" component="h3">
                            Absolute time range
                        </Typography>
                        <TextField
                            label="From"
                            value={draftFrom}
                            onChange={(event) => setDraftFrom(event.target.value)}
                            size="small"
                            fullWidth
                            autoFocus
                        />
                        <TextField
                            label="To"
                            value={draftTo}
                            onChange={(event) => setDraftTo(event.target.value)}
                            size="small"
                            fullWidth
                        />
                        <Stack direction="row" spacing={1} sx={{alignItems: 'center', flexWrap: 'wrap'}}>
                            <Button
                                variant="outlined"
                                size="small"
                                aria-label="Copy time range"
                                title="Copy time range"
                                onClick={handleCopy}
                                sx={{minWidth: 40, width: 40, px: 0}}
                            >
                                <ContentCopyIcon fontSize="small" />
                            </Button>
                            <Button
                                variant="outlined"
                                size="small"
                                aria-label="Paste time range"
                                title="Paste time range"
                                onClick={handlePaste}
                                sx={{minWidth: 40, width: 40, px: 0}}
                            >
                                <ContentPasteIcon fontSize="small" />
                            </Button>
                            <Button
                                variant="contained"
                                onClick={() => applyRange(draftRange)}
                                sx={{minWidth: 144}}
                            >
                                Apply time range
                            </Button>
                        </Stack>
                        {error && <Alert severity="error">{error}</Alert>}
                        {status && !error && (
                            <Typography variant="caption" color="success.main">
                                {status}
                            </Typography>
                        )}
                    </Stack>
                    <Stack spacing={1} sx={{p: 2.5, flex: '1 1 44%', minWidth: 0}}>
                        <TextField
                            value={search}
                            onChange={(event) => setSearch(event.target.value)}
                            placeholder="Search quick ranges"
                            size="small"
                            InputProps={{
                                startAdornment: (
                                    <InputAdornment position="start">
                                        <SearchRoundedIcon fontSize="small" />
                                    </InputAdornment>
                                ),
                            }}
                        />
                        <List dense disablePadding sx={{mx: -1}}>
                            {filteredQuickRanges.map((item) => (
                                <ListItemButton
                                    key={item.label}
                                    selected={selectedQuickRange?.label === item.label}
                                    onClick={() => applyRange(item.range)}
                                    sx={{borderRadius: 1}}
                                >
                                    <Typography variant="body1">{item.label}</Typography>
                                </ListItemButton>
                            ))}
                        </List>
                    </Stack>
                </Stack>
                <Box
                    sx={{
                        px: 2.5,
                        py: 1.5,
                        borderTop: (theme) => `1px solid ${theme.palette.divider}`,
                    }}
                >
                    <Typography variant="body2" color="text.secondary">
                        Browser Time {timeZoneLabel}
                    </Typography>
                </Box>
            </Popover>
        </>
    );
}
