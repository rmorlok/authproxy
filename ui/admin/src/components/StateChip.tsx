import {ConnectorReleaseState} from "@authproxy/api";
import Chip from "@mui/material/Chip";
import React from "react";

export function StateChip({state}: { state: ConnectorReleaseState }) {
    const colors: Record<ConnectorReleaseState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [ConnectorReleaseState.DRAFT]: 'secondary',
        [ConnectorReleaseState.PRIMARY]: 'primary',
        [ConnectorReleaseState.ACTIVE]: 'info',
        [ConnectorReleaseState.ARCHIVED]: 'default',
    };
    return <Chip label={state} color={colors[state]} size="small"/>;
}
