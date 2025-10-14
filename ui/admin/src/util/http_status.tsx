import Chip from "@mui/material/Chip";
import * as React from "react";

export function HttpStatusChip({value}: {value: number}) {
    let color: "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary" = "default";
    if (value >= 200 && value < 300) {
        color = "success";
    } else if (value >= 400 && value < 500) {
        color = "warning";
    } else if (value >= 500) {
        color = "error";
    }

    return <Chip label={value} color={color} size="small" />;
}