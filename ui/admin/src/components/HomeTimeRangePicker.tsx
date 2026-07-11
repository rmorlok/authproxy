import * as React from 'react';
import dayjs, {Dayjs} from 'dayjs';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import ButtonBase from '@mui/material/ButtonBase';
import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import InputAdornment from '@mui/material/InputAdornment';
import List from '@mui/material/List';
import ListItemButton from '@mui/material/ListItemButton';
import Popover from '@mui/material/Popover';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import AccessTimeRoundedIcon from '@mui/icons-material/AccessTimeRounded';
import CalendarTodayRoundedIcon from '@mui/icons-material/CalendarTodayRounded';
import ChevronLeftRoundedIcon from '@mui/icons-material/ChevronLeftRounded';
import ChevronRightRoundedIcon from '@mui/icons-material/ChevronRightRounded';
import CloseRoundedIcon from '@mui/icons-material/CloseRounded';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import ContentPasteIcon from '@mui/icons-material/ContentPaste';
import SearchRoundedIcon from '@mui/icons-material/SearchRounded';
import {
    browserTimeZoneLabel,
    calendarRangeFromDashboardRange,
    describeDashboardTimeRange,
    formatCalendarRangeEnd,
    formatCalendarRangeStart,
    parseGrafanaTimeRange,
    QUICK_DASHBOARD_TIME_RANGES,
    rangesEqual,
    serializeGrafanaTimeRange,
    timeRangeValidationError,
} from '../metrics/timeRange';
import type {DashboardCalendarRange, DashboardTimeRange} from '../metrics/timeRange';

type RangePosition = 'start' | 'end';

interface HomeTimeRangePickerProps {
    value: DashboardTimeRange;
    onApply: (range: DashboardTimeRange) => void;
}

const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

export default function HomeTimeRangePicker({value, onApply}: HomeTimeRangePickerProps) {
    const [anchorEl, setAnchorEl] = React.useState<HTMLElement | null>(null);
    const [draftFrom, setDraftFrom] = React.useState(value.from);
    const [draftTo, setDraftTo] = React.useState(value.to);
    const [search, setSearch] = React.useState('');
    const [error, setError] = React.useState<string | null>(null);
    const [status, setStatus] = React.useState<string | null>(null);
    const [calendarOpen, setCalendarOpen] = React.useState(false);
    const [calendarRange, setCalendarRange] = React.useState<DashboardCalendarRange>(
        () => calendarRangeFromDashboardRange(value),
    );
    const [rangePosition, setRangePosition] = React.useState<RangePosition>('start');
    const [calendarMonth, setCalendarMonth] = React.useState(() => calendarMonthForRange(
        calendarRangeFromDashboardRange(value),
    ));
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
    const calendarDays = React.useMemo(() => calendarGridDays(calendarMonth), [calendarMonth]);

    React.useEffect(() => {
        if (open) {
            setDraftFrom(value.from);
            setDraftTo(value.to);
            setSearch('');
            setError(null);
            setStatus(null);
            setCalendarOpen(false);
            const nextCalendarRange = calendarRangeFromDashboardRange(value);
            setCalendarRange(nextCalendarRange);
            setCalendarMonth(calendarMonthForRange(nextCalendarRange));
            setRangePosition('start');
        }
    }, [open, value]);

    const close = () => {
        setAnchorEl(null);
        setCalendarOpen(false);
    };

    const openCalendar = (position: RangePosition) => {
        const nextCalendarRange = calendarRangeFromDashboardRange(draftRange);
        setCalendarRange(nextCalendarRange);
        setCalendarMonth(calendarMonthForRange(nextCalendarRange, position));
        setRangePosition(position);
        setCalendarOpen(true);
        setError(null);
        setStatus(null);
    };

    const updateDraftFromCalendar = (nextRange: DashboardCalendarRange) => {
        setCalendarRange(nextRange);
        setError(null);
        setStatus(null);

        const [from, to] = nextRange;
        if (from) {
            setDraftFrom(formatCalendarRangeStart(from));
        }
        if (to) {
            setDraftTo(formatCalendarRangeEnd(to));
        }
    };

    const selectCalendarDay = (day: Dayjs) => {
        const [currentFrom, currentTo] = calendarRange;
        let nextRange: DashboardCalendarRange;

        if (rangePosition === 'start') {
            nextRange = currentTo && !day.isAfter(currentTo, 'day')
                ? [day, currentTo]
                : [day, day];
            setRangePosition('end');
        } else if (currentFrom && day.isBefore(currentFrom, 'day')) {
            nextRange = [day, currentFrom];
            setRangePosition('start');
        } else {
            nextRange = [currentFrom ?? day, day];
            setRangePosition('start');
        }

        updateDraftFromCalendar(nextRange);
    };

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
            const nextCalendarRange = calendarRangeFromDashboardRange(parsed);
            setCalendarRange(nextCalendarRange);
            setCalendarMonth(calendarMonthForRange(nextCalendarRange));
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
                        width: {xs: 'calc(100vw - 32px)', md: calendarOpen ? 1180 : 780},
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
                    {calendarOpen && (
                        <Stack spacing={1.5} sx={{p: 2.5, flex: '0 0 340px', minWidth: 0}}>
                            <Stack direction="row" sx={{alignItems: 'center', justifyContent: 'space-between'}}>
                                <Typography variant="h6" component="h3">
                                    Select a time range
                                </Typography>
                                <Tooltip title="Close calendar">
                                    <IconButton
                                        aria-label="Close calendar"
                                        size="small"
                                        onClick={() => setCalendarOpen(false)}
                                    >
                                        <CloseRoundedIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                            <Stack spacing={1.5}>
                                <Stack direction="row" sx={{alignItems: 'center', justifyContent: 'space-between'}}>
                                    <IconButton
                                        aria-label="Previous month"
                                        size="small"
                                        onClick={() => setCalendarMonth((month) => month.subtract(1, 'month'))}
                                    >
                                        <ChevronLeftRoundedIcon />
                                    </IconButton>
                                    <Typography variant="subtitle1" sx={{fontWeight: 700}}>
                                        {calendarMonth.format('MMMM YYYY')}
                                    </Typography>
                                    <IconButton
                                        aria-label="Next month"
                                        size="small"
                                        onClick={() => setCalendarMonth((month) => month.add(1, 'month'))}
                                    >
                                        <ChevronRightRoundedIcon />
                                    </IconButton>
                                </Stack>
                                <Box
                                    sx={{
                                        display: 'grid',
                                        gridTemplateColumns: 'repeat(7, minmax(0, 1fr))',
                                        gap: 0.5,
                                    }}
                                >
                                    {WEEKDAYS.map((weekday) => (
                                        <Typography
                                            key={weekday}
                                            variant="caption"
                                            color="primary"
                                            sx={{textAlign: 'center', fontWeight: 700}}
                                        >
                                            {weekday}
                                        </Typography>
                                    ))}
                                    {calendarDays.map((day) => {
                                        const isOutsideMonth = !day.isSame(calendarMonth, 'month');
                                        const isRangeStart = isSameCalendarDay(day, calendarRange[0]);
                                        const isRangeEnd = isSameCalendarDay(day, calendarRange[1]);
                                        const isInRange = isDayInCalendarRange(day, calendarRange);
                                        const isSelected = isRangeStart || isRangeEnd;

                                        return (
                                            <ButtonBase
                                                key={day.format('YYYY-MM-DD')}
                                                aria-label={`Select ${day.format('MMM D, YYYY')}`}
                                                onClick={() => selectCalendarDay(day)}
                                                sx={{
                                                    height: 40,
                                                    borderRadius: isSelected ? 5 : 1,
                                                    color: isOutsideMonth ? 'text.disabled' : 'text.primary',
                                                    bgcolor: isSelected
                                                        ? 'primary.main'
                                                        : isInRange
                                                            ? 'action.selected'
                                                            : 'transparent',
                                                    fontWeight: isSelected ? 700 : 500,
                                                    '&:hover': {
                                                        bgcolor: isSelected ? 'primary.dark' : 'action.hover',
                                                    },
                                                }}
                                            >
                                                {day.date()}
                                            </ButtonBase>
                                        );
                                    })}
                                </Box>
                            </Stack>
                        </Stack>
                    )}
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
                            InputProps={{
                                endAdornment: (
                                    <InputAdornment position="end">
                                        <Tooltip title="Select from calendar">
                                            <IconButton
                                                aria-label="Select From date range"
                                                edge="end"
                                                onMouseDown={(event) => event.preventDefault()}
                                                onClick={() => openCalendar('start')}
                                            >
                                                <CalendarTodayRoundedIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    </InputAdornment>
                                ),
                            }}
                        />
                        <TextField
                            label="To"
                            value={draftTo}
                            onChange={(event) => setDraftTo(event.target.value)}
                            size="small"
                            fullWidth
                            InputProps={{
                                endAdornment: (
                                    <InputAdornment position="end">
                                        <Tooltip title="Select from calendar">
                                            <IconButton
                                                aria-label="Select To date range"
                                                edge="end"
                                                onMouseDown={(event) => event.preventDefault()}
                                                onClick={() => openCalendar('end')}
                                            >
                                                <CalendarTodayRoundedIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    </InputAdornment>
                                ),
                            }}
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

function calendarMonthForRange(range: DashboardCalendarRange, position: RangePosition = 'start'): Dayjs {
    const preferredDate = position === 'end' ? range[1] ?? range[0] : range[0] ?? range[1];
    return (preferredDate ?? dayjs()).startOf('month');
}

function calendarGridDays(month: Dayjs): Dayjs[] {
    const firstVisibleDay = startOfMondayWeek(month.startOf('month'));
    return Array.from({length: 42}, (_, index) => firstVisibleDay.add(index, 'day'));
}

function startOfMondayWeek(date: Dayjs): Dayjs {
    const daysSinceMonday = (date.day() + 6) % 7;
    return date.subtract(daysSinceMonday, 'day').startOf('day');
}

function isSameCalendarDay(a: Dayjs, b: Dayjs | null): boolean {
    return Boolean(b?.isSame(a, 'day'));
}

function isDayInCalendarRange(day: Dayjs, range: DashboardCalendarRange): boolean {
    const [from, to] = range;
    if (!from || !to) {
        return false;
    }
    return (
        day.isSame(from, 'day') ||
        day.isSame(to, 'day') ||
        (day.isAfter(from, 'day') && day.isBefore(to, 'day'))
    );
}
