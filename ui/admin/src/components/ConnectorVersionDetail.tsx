import React, {useEffect, useMemo, useState} from 'react';
import Box from '@mui/material/Box';
import { useTheme } from '@mui/material/styles';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import Stack from '@mui/material/Stack';
import Avatar from '@mui/material/Avatar';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import Menu from '@mui/material/Menu';
import MenuItem from '@mui/material/MenuItem';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import FormHelperText from '@mui/material/FormHelperText';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import MuiLink from '@mui/material/Link';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import ToggleButton from '@mui/material/ToggleButton';
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup';
import {connectors, Connector, ConnectorReleaseState} from '@authproxy/api';
import AnnotationsEditor from "./AnnotationsEditor";
import YAML from 'yaml';
import {StateChip} from "./StateChip";
import CodeMirror from "@uiw/react-codemirror";
import { yaml as yamlMode } from "@codemirror/lang-yaml";
import { json as jsonMode } from "@codemirror/lang-json";
import { oneDark } from "@codemirror/theme-one-dark";

interface AdminConnectorDefinition extends Record<string, unknown> {
    displayName?: string;
    description?: string;
    highlight?: string;
    logo?: {publicUrl?: string; base64?: string; mimeType?: string};
    statusPageUrl?: string;
    marketplaceUrl?: string;
    developerConsoleUrl?: string;
    oauthClientUrl?: string;
}

function getLogoUrlFromDefinition(definition?: AdminConnectorDefinition): string {
    if (!definition?.logo) return "";

    if (definition.logo.publicUrl) {
        return definition.logo.publicUrl;
    }

    if (definition.logo.base64) {
        if (definition.logo.mimeType === "image/svg+xml") {
            return `data:${definition.logo.mimeType};base64,${definition.logo.base64}`;
        } else {
            return `data:image/png;base64,${definition.logo.base64}`;
        }
    }

    return "";
}

export default function ConnectorVersionDetail(
    { connectorId, version, connectorVersion}: ({ connectorId?: string, version?: number, connectorVersion?: Connector})
) {
    const theme = useTheme();
    const [loading, setLoading] = useState(!connectorVersion);
    const [error, setError] = useState<string | null>(null);
    const [cv, setCv] = useState<Connector | null>(connectorVersion || null);
    const definition = cv?.spec.definition as AdminConnectorDefinition | undefined;

    // versions state
    const [viewMode, setViewMode] = useState<'json' | 'yaml' | 'visual'>('yaml');
    const [definitionFormatted, setDefinitionFormatted] = React.useState("");
    const [langMode, setLangMode] = React.useState(yamlMode);

    // Force state UI
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [forceStateOpen, setForceStateOpen] = useState(false);
    const [selectedState, setSelectedState] = useState<ConnectorReleaseState | ''>('');
    const [actionLoading, setActionLoading] = useState(false);
    const [actionError, setActionError] = useState<string | null>(null);

    const stateOptions = useMemo(() => Object.values(ConnectorReleaseState), []);

    const fetchConnectorVersion = () => {
        if (!connectorId || !version) return;
        setLoading(true);
        setError(null);
        connectors.getGeneration(connectorId, version)
            .then(res => {
                setCv(res.data);
            })
            .catch(err => {
                const msg = err?.response?.data?.error || err.message || 'Failed to load connector';
                setError(msg);
            })
            .finally(() => setLoading(false));
    };

    useEffect(() => {
        if (!definition) {
            setDefinitionFormatted("")
            setLangMode(yamlMode);
        } else if (viewMode === 'json') {
            setDefinitionFormatted(JSON.stringify(definition, null, 2));
            setLangMode(jsonMode);
        } else {
            setDefinitionFormatted(YAML.stringify(definition));
            setLangMode(yamlMode);
        }
    }, [viewMode, definition]);

    useEffect(() => {
        if (cv || !connectorId || !version) return;
        let cancelled = false;
        setLoading(true);
        setError(null);
        connectors.getGeneration(connectorId, version)
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

    const openMenu = (e: React.MouseEvent<HTMLButtonElement>) => setMenuAnchorEl(e.currentTarget);
    const closeMenu = () => setMenuAnchorEl(null);

    const onClickForceState = () => {
        setActionError(null);
        setSelectedState(cv.status.release.state);
        closeMenu();
        setForceStateOpen(true);
    };

    const onSubmitForceState = async () => {
        if (!cv || !selectedState) return;
        setActionError(null);
        setActionLoading(true);
        try {
            await connectors.forceGenerationState(cv.metadata.id, cv.metadata.generation, selectedState);
            setForceStateOpen(false);
            fetchConnectorVersion();
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to force state';
            setActionError(msg);
        } finally {
            setActionLoading(false);
        }
    };

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
                {definition?.description && (
                    <Typography variant="body1" color="text.secondary">{definition.description}</Typography>
                )}

                {definition?.highlight && (
                    <Alert severity="info">{definition.highlight}</Alert>
                )}
            </Box>
        );
    }

    return (
        <Stack spacing={2} sx={{p: 2}}>
            <Stack direction="row" spacing={2} alignItems="center">
                {definition?.logo &&
                    <Avatar alt={definition.displayName} src={getLogoUrlFromDefinition(definition)} sx={{width: 40, height: 40}}/>}
                <Typography variant="h5">{definition?.displayName || cv.metadata.labels?.type || 'Unnamed Connector'}</Typography>
                <StateChip state={cv.status.release.state}/>
                <IconButton aria-label="actions" onClick={openMenu} size="small">
                    <MoreVertIcon/>
                </IconButton>
                <Menu anchorEl={menuAnchorEl} open={Boolean(menuAnchorEl)} onClose={closeMenu}>
                    <MenuItem onClick={onClickForceState}>Force state…</MenuItem>
                </Menu>
            </Stack>

            {actionError && <Alert severity="error">{actionError}</Alert>}

            {(definition?.statusPageUrl || definition?.marketplaceUrl || definition?.developerConsoleUrl || definition?.oauthClientUrl) && (
                <Stack direction="row" spacing={2} flexWrap="wrap">
                    {definition.statusPageUrl && (
                        <MuiLink href={definition.statusPageUrl} target="_blank" rel="noopener noreferrer" underline="hover" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
                            Status Page <OpenInNewIcon fontSize="inherit" />
                        </MuiLink>
                    )}
                    {definition.marketplaceUrl && (
                        <MuiLink href={definition.marketplaceUrl} target="_blank" rel="noopener noreferrer" underline="hover" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
                            Marketplace <OpenInNewIcon fontSize="inherit" />
                        </MuiLink>
                    )}
                    {definition.developerConsoleUrl && (
                        <MuiLink href={definition.developerConsoleUrl} target="_blank" rel="noopener noreferrer" underline="hover" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
                            Developer Console <OpenInNewIcon fontSize="inherit" />
                        </MuiLink>
                    )}
                    {definition.oauthClientUrl && (
                        <MuiLink href={definition.oauthClientUrl} target="_blank" rel="noopener noreferrer" underline="hover" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
                            OAuth Client <OpenInNewIcon fontSize="inherit" />
                        </MuiLink>
                    )}
                </Stack>
            )}

            <Stack direction={{xs: 'column', sm: 'row'}} spacing={4}>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Connector ID</Typography>
                    <Typography variant="body1" sx={{wordBreak: 'break-all'}}>{cv.metadata.id}</Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Labels</Typography>
                    {cv.metadata.labels && Object.keys(cv.metadata.labels).length > 0 ? (
                        <Stack direction="row" spacing={0.5} flexWrap="wrap" sx={{ mt: 0.5 }}>
                            {Object.entries(cv.metadata.labels).map(([key, value]) => (
                                <Chip key={key} label={`${key}: ${value}`} size="small" variant="outlined" />
                            ))}
                        </Stack>
                    ) : (
                        <Typography variant="body2" color="text.secondary">No labels</Typography>
                    )}
                </Box>
                <Box>
                    <Typography variant="subtitle2" color="text.secondary">Version</Typography>
                    <Typography variant="body1">{cv.metadata.generation}</Typography>
                </Box>
            </Stack>

            <AnnotationsEditor
                annotations={cv.metadata.annotations}
                readOnly={cv.status.release.state !== ConnectorReleaseState.DRAFT}
                onPut={async (key, value) => {
                    await connectors.putGenerationAnnotation(cv.metadata.id, cv.metadata.generation, key, value);
                    if (connectorId && version) {
                        fetchConnectorVersion();
                    } else {
                        const res = await connectors.getGeneration(cv.metadata.id, cv.metadata.generation);
                        setCv(res.data);
                    }
                }}
                onDelete={async (key) => {
                    await connectors.deleteGenerationAnnotation(cv.metadata.id, cv.metadata.generation, key);
                    if (connectorId && version) {
                        fetchConnectorVersion();
                    } else {
                        const res = await connectors.getGeneration(cv.metadata.id, cv.metadata.generation);
                        setCv(res.data);
                    }
                }}
            />

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

            {/* Force state dialog */}
            <Dialog open={forceStateOpen} onClose={() => !actionLoading && setForceStateOpen(false)} fullWidth maxWidth="sm">
                <DialogTitle>Force connector version state</DialogTitle>
                <DialogContent>
                    <FormControl fullWidth sx={{mt: 2}}>
                        <InputLabel id="force-cv-state-label">State</InputLabel>
                        <Select
                            native
                            labelId="force-cv-state-label"
                            label="State"
                            value={selectedState || ''}
                            onChange={(e) => setSelectedState((e.target as HTMLSelectElement).value as ConnectorReleaseState)}
                        >
                            <option aria-label="None" value="" />
                            {stateOptions.map(s => (
                                <option key={s} value={s}>{s}</option>
                            ))}
                        </Select>
                        <FormHelperText>Select the state to force for this connector version.</FormHelperText>
                    </FormControl>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setForceStateOpen(false)} disabled={actionLoading}>Cancel</Button>
                    <Button onClick={onSubmitForceState} variant="contained" disabled={!selectedState || actionLoading}>Apply</Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );
}
