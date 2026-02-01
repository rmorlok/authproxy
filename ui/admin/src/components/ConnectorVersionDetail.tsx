import React, {useEffect, useState} from 'react';
import Box from '@mui/material/Box';
import { useTheme } from '@mui/material/styles';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Avatar from '@mui/material/Avatar';
import Chip from '@mui/material/Chip';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import {connectors, ConnectorVersion} from '@authproxy/api';
import YAML from 'yaml';
import {StateChip} from "./StateChip";
import CodeMirror from "@uiw/react-codemirror";
import { yaml as yamlMode } from "@codemirror/lang-yaml";
import { json as jsonMode } from "@codemirror/lang-json";
import { oneDark } from "@codemirror/theme-one-dark";

function getLogoUrlFromDefinition(cv: ConnectorVersion): string {
    if (!cv || !cv.definition || !cv.definition.logo) return "";

    if (cv.definition.logo.public_url) {
        return cv.definition.logo.public_url;
    }

    if (cv.definition.logo.base64) {
        if (cv.definition.logo.mime_type === "image/svg+xml") {
            return `data:${cv.definition.logo.mime_type};base64,${cv.definition.logo.base64}`;
        } else {
            return `data:image/png;base64,${cv.definition.logo.base64}`;
        }
    }

    return "";
}

export default function ConnectorVersionDetail(
    { connectorId, version, connectorVersion}: ({ connectorId?: string, version?: number, connectorVersion?: ConnectorVersion})
) {
    const theme = useTheme();
    const [loading, setLoading] = useState(!connectorVersion);
    const [error, setError] = useState<string | null>(null);
    const [cv, setCv] = useState<ConnectorVersion | null>(connectorVersion || null);

    // versions state
    const [viewMode, setViewMode] = useState<'json' | 'yaml' | 'visual'>('yaml');
    const [definitionFormatted, setDefinitionFormatted] = React.useState("");
    const [langMode, setLangMode] = React.useState(yamlMode);

    useEffect(() => {
        if (!cv?.definition) {
            setDefinitionFormatted("")
            setLangMode(yamlMode);
        } else if (viewMode === 'json') {
            setDefinitionFormatted(JSON.stringify(cv?.definition, null, 2));
            setLangMode(jsonMode);
        } else {
            setDefinitionFormatted(YAML.stringify(cv.definition as any));
            setLangMode(yamlMode);
        }
    }, [viewMode, cv?.definition]);

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
                <CodeMirror
                    value={definitionFormatted}
                    theme={theme.palette.mode === 'dark' ? oneDark : undefined}
                    extensions={[langMode]}
                    editable={false}
                />
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
                    <Avatar alt={cv.definition.display_name} src={getLogoUrlFromDefinition(cv)} sx={{width: 40, height: 40}}/>}
                <Typography variant="h5">{cv.definition.display_name || cv.labels?.type || 'Unnamed Connector'}</Typography>
                <StateChip state={cv.state}/>
            </Stack>

            <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Connector ID</Typography>
                    <Typography variant="body1" sx={{wordBreak: 'break-all'}}>{cv.id}</Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
                    {cv.labels && Object.keys(cv.labels).length > 0 ? (
                        <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
                            {Object.entries(cv.labels).map(([key, value]) => (
                                <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
                            ))}
                        </Stack>
                    ) : (
                        <Typography variant="body2" color="text.secondary">No labels</Typography>
                    )}
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Version</Typography>
                    <Typography variant="body1">{cv.version}</Typography>
                </Box>
            </Stack>

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
        </Stack>
    );
}
