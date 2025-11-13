import React, {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Avatar from '@mui/material/Avatar';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import {connectors, ConnectorVersion} from '@authproxy/api';
import YAML from 'yaml';
import {useNavigate} from 'react-router-dom';
import {StateChip} from "./StateChip";

export default function ConnectorVersionDetail(
    { connectorId, version, connectorVersion}: ({ connectorId?: string, version?: number, connectorVersion?: ConnectorVersion})
) {
    const [loading, setLoading] = useState(!connectorVersion);
    const [error, setError] = useState<string | null>(null);
    const [cv, setCv] = useState<ConnectorVersion | null>(connectorVersion || null);

    // versions state
    const [viewMode, setViewMode] = useState<'json' | 'yaml' | 'visual'>('yaml');
    const navigate = useNavigate();

    useEffect(() => {
        if (cv || !connectorId || !version) return;
        let cancelled = false;
        setLoading(true);
        setError(null);
        connectors.getVersion(connectorId, version)
            .then(res => {
                if (cancelled) return;
                setCv(res.data);
            })
            .catch(err => {
                if (cancelled) return;
                const msg = err?.response?.data?.error || err.message || 'Failed to load connector';
                setError(msg);
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [connectorId, version]);

    if (loading) return (<Box sx={{display: 'flex', justifyContent: 'center', p: 4}}><CircularProgress/></Box>);
    if (error) return (<Alert severity="error">{error}</Alert>);
    if (!cv) return null;

    function preformattedRendering() {
        if(!cv) {
            return null;
        }

        return (
            <Box sx={{
                flex: 1,
                overflow: 'auto',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
                p: 1
            }}>
            <pre style={{margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word'}}>
{`
${viewMode === 'json' ? JSON.stringify(cv.definition, null, 2) : YAML.stringify(cv.definition as any)}
`}
            </pre>
            </Box>
        );
    }

    function visualRendering() {
        if (!cv) {
            return null;
        }
        return (
            <Box sx={{
                flex: 1,
                overflow: 'auto',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1,
                p: 1
            }}>
                {cv.definition.description && (
                    <Typography variant="body1" color="text.secondary">{cv.definition.description}</Typography>
                )}

                {cv.definition.highlight && (
                    <Alert severity="info">{cv.definition.highlight}</Alert>
                )}
            </Box>
        );
    }

    return (
        <Stack spacing={2} sx={{p: 2}}>
            <Stack direction="row" spacing={2} alignItems="center">
                {cv.definition.logo &&
                    <Avatar alt={cv.definition.display_name} src={cv.definition.logo} sx={{width: 40, height: 40}}/>}
                <Typography variant="h5">{cv.definition.display_name || cv.type}</Typography>
                <StateChip state={cv.state}/>
            </Stack>

            <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Connector ID</Typography>
                    <Typography variant="body1" sx={{wordBreak: 'break-all'}}>{cv.id}</Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Type</Typography>
                    <Typography variant="body1">{cv.type}</Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Version</Typography>
                    <Typography variant="body1">{cv.version}</Typography>
                </Box>
            </Stack>

            <Box sx={{p: 2, height: '100%', display: 'flex', flexDirection: 'column'}}>
                <Box sx={{mt: 1, mb: 1}}>
                    <ToggleButtonGroup
                        size="small"
                        value={viewMode}
                        exclusive
                        onChange={(_, val) => {
                            if (val) setViewMode(val);
                        }}
                    >
                        <ToggleButton value="yaml">YAML</ToggleButton>
                        <ToggleButton value="json">JSON</ToggleButton>
                        <ToggleButton value="visual">Visual</ToggleButton>
                    </ToggleButtonGroup>
                </Box>
                {viewMode === 'visual' ? visualRendering() : preformattedRendering()}
            </Box>
        </Stack>
    );
}
