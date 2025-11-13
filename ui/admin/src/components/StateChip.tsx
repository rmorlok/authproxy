import {ConnectorVersionState} from "@authproxy/api";
import Chip from "@mui/material/Chip";
import React from "react";

export function StateChip({state}: { state: ConnectorVersionState }) {
    const colors: Record<ConnectorVersionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [ConnectorVersionState.DRAFT]: 'secondary',
        [ConnectorVersionState.PRIMARY]: 'primary',
        [ConnectorVersionState.ACTIVE]: 'info',
        [ConnectorVersionState.ARCHIVED]: 'default',
    };
    return <Chip label={state} color={colors[state]} size="small"/>;
}