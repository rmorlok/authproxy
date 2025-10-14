import {Tooltip} from "@mui/material";
import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";

dayjs.extend(duration);

/**
 * Format milliseconds into a friendly time string
 * @param ms Time in milliseconds
 * @returns Formatted string (e.g. "250ms", "2.5s", "1min 30s", "2hr")
 */
export function formatDuration(ms: number): string {
    if (ms < 1000) {
        return `${ms}ms`;
    }

    const duration = dayjs.duration(ms);

    if (ms < 60 * 1000) {
        // Less than a minute
        return `${duration.asSeconds().toFixed(1)}s`;
    }

    if (ms < 60 * 60 * 1000) {
        // Less than an hour
        const minutes = Math.floor(duration.asMinutes());
        const seconds = duration.seconds();
        return seconds > 0 ? `${minutes}min ${seconds}s` : `${minutes}min`;
    }

    if (ms < 24 * 60 * 60 * 1000) {
        // Less than a day
        const hours = Math.floor(duration.asHours());
        const minutes = duration.minutes();
        return minutes > 0 ? `${hours}hr ${minutes}min` : `${hours}hr`;
    }

    // A day or more
    const days = Math.floor(duration.asDays());
    const hours = duration.hours();
    return hours > 0 ? `${days}d ${hours}hr` : `${days}d`;
}

export function Duration({value}: {value: number}) {
    return (<Tooltip title={value + "ms"}>
        <span>{formatDuration(value)}</span>
    </Tooltip>);
}