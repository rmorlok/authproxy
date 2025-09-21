import * as React from 'react';
import Grid from '@mui/material/Grid';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import {rows} from "../data/gridData";
import {DataGrid, GridColDef} from "@mui/x-data-grid";
import Chip from "@mui/material/Chip";
import {ConnectionState} from "../api";

function renderState(state: ConnectionState) {
    const colors: Record<ConnectionState, "default" | "success" | "error" | "info" | "warning" | "primary" | "secondary"> = {
        [ConnectionState.CREATED]: 'default',
        [ConnectionState.CONNECTED]: 'success',
        [ConnectionState.FAILED]: 'error',
        [ConnectionState.DISCONNECTING]: 'info',
        [ConnectionState.DISCONNECTED]: 'warning'
    };

    return <Chip label={state} color={colors[state]} size="small" />;
}

export const columns: GridColDef[] = [
    { field: 'id',
        headerName: 'ID',
        flex: 1.5,
        minWidth: 200
    },
    {
        field: 'state',
        headerName: 'State',
        flex: 0.5,
        minWidth: 80,
        renderCell: (params) => renderState(params.value as ConnectionState),
    },
    {
        field: 'created_at',
        headerName: 'Created At',
        headerAlign: 'right',
        align: 'right',
        flex: 1,
        minWidth: 80,
    },
    {
        field: 'updated_at',
        headerName: 'Updated At',
        headerAlign: 'right',
        align: 'right',
        flex: 1,
        minWidth: 100,
    },
];

export default function Connections() {
    return (
        <Box sx={{width: '100%', maxWidth: {sm: '100%', md: '1700px'}}}>
            <Typography component="h2" variant="h6" sx={{mb: 2}}>
                Connections
            </Typography>
            <Grid size={{xs: 12, lg: 9}}>
                <DataGrid
                    checkboxSelection
                    rows={rows}
                    columns={columns}
                    getRowClassName={(params) =>
                        params.indexRelativeToCurrentPage % 2 === 0 ? 'even' : 'odd'
                    }
                    initialState={{
                        pagination: { paginationModel: { pageSize: 20 } },
                    }}
                    pageSizeOptions={[10, 20, 50]}
                    disableColumnResize
                    density="compact"
                    slotProps={{
                        filterPanel: {
                            filterFormProps: {
                                logicOperatorInputProps: {
                                    variant: 'outlined',
                                    size: 'small',
                                },
                                columnInputProps: {
                                    variant: 'outlined',
                                    size: 'small',
                                    sx: { mt: 'auto' },
                                },
                                operatorInputProps: {
                                    variant: 'outlined',
                                    size: 'small',
                                    sx: { mt: 'auto' },
                                },
                                valueInputProps: {
                                    InputComponentProps: {
                                        variant: 'outlined',
                                        size: 'small',
                                    },
                                },
                            },
                        },
                    }}
                />
            </Grid>
        </Box>
    );
}
