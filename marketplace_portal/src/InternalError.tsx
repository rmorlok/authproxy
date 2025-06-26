import Typography from "@mui/material/Typography";
import Link from "@mui/material/Link";
import * as React from "react";

export function InternalError(props: any) {
    return (
        <Typography variant="body2" color="text.secondary" align="center" {...props}>
            {'Internal Server Error'}
        </Typography>
    );
}