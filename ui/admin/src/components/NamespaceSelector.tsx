import * as React from 'react';
import MuiAvatar from '@mui/material/Avatar';
import MuiListItemAvatar from '@mui/material/ListItemAvatar';
import MenuItem from '@mui/material/MenuItem';
import ListItemText from '@mui/material/ListItemText';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListSubheader from '@mui/material/ListSubheader';
import Select, {SelectChangeEvent, selectClasses} from '@mui/material/Select';
import Divider from '@mui/material/Divider';
import {styled} from '@mui/material/styles';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import TextField from '@mui/material/TextField';
import AddRoundedIcon from '@mui/icons-material/AddRounded';
import RefreshIcon from '@mui/icons-material/Refresh';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import ErrorIcon from '@mui/icons-material/Error';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import FolderIcon from '@mui/icons-material/Folder';
import {useDispatch, useSelector} from "react-redux";
import {
    selectCurrentNamespace,
    selectCurrentNamespaceChildren,
    selectNamespaceChildrenStatus,
    selectNamespaceStatus, setCurrentNamespace
} from "../store/namespacesSlice";
import CircularProgress from "@mui/material/CircularProgress";
import {AppDispatch} from "../store";
import {useEffect} from "react";
import {NAMESPACE_PATH_SEPARATOR, namespaces, ROOT_NAMESPACE_PATH} from "@authproxy/api";

const ACTION_ADD = "...action.add";
const ACTION_CHILDREN = "...action.children";
const ACTION_LOADING_NAMESPACE = "...action.loading-ns";
const ACTION_NAVIGATE_PARENT = "...action.navigate-parent";
const ACTION_NAVIGATE_ROOT = "...action.navigate-root";
const ACTION_REFRESH = "...action.refresh";
const ACTION_PREFIX = "...action.";
const NAMESPACE_LEAF_REGEX = /^[A-Za-z0-9_][A-Za-z0-9_-]*$/;

const Avatar = styled(MuiAvatar)(({theme}) => ({
    width: 28,
    height: 28,
    backgroundColor: (theme.vars || theme).palette.background.paper,
    color: (theme.vars || theme).palette.text.secondary,
    border: `1px solid ${(theme.vars || theme).palette.divider}`,
}));

const ListItemAvatar = styled(MuiListItemAvatar)({
    minWidth: 0,
    marginRight: 12,
});

function depth(path: string | null | undefined): number {
    if (!path) {
        return 0;
    }

    return path.split(NAMESPACE_PATH_SEPARATOR).length - 1;
}

function parentNamespace(path: string | null | undefined): string {
    if (!path) {
        return ROOT_NAMESPACE_PATH;
    }

    const parts = path.split(NAMESPACE_PATH_SEPARATOR);
    if (parts.length > 1) {
        return parts.slice(0, -1).join(NAMESPACE_PATH_SEPARATOR);
    } else {
        return ROOT_NAMESPACE_PATH;
    }
}

function leafNamespace(path: string | null | undefined): string {
    if (!path) {
        return "";
    }

    const parts = path.split(NAMESPACE_PATH_SEPARATOR);
    if (parts.length > 1) {
        return parts[parts.length-1];
    } else {
        return "";
    }
}

export function childNamespacePath(parentPath: string | null | undefined, leafName: string): string {
    const parent = parentPath || ROOT_NAMESPACE_PATH;
    return `${parent}${NAMESPACE_PATH_SEPARATOR}${leafName}`;
}

export function validateNamespaceLeafName(leafName: string): string | null {
    if (!leafName) {
        return "Name is required";
    }

    if (leafName.includes(NAMESPACE_PATH_SEPARATOR)) {
        return "Enter only the child namespace name";
    }

    if (!NAMESPACE_LEAF_REGEX.test(leafName)) {
        return "Use letters, numbers, underscores, and hyphens";
    }

    return null;
}

export default function NamespaceSelector() {
    const dispatch = useDispatch<AppDispatch>();
    const loadingStatus = useSelector(selectNamespaceStatus);
    const childLoadingStatus = useSelector(selectNamespaceChildrenStatus);
    const ns = useSelector(selectCurrentNamespace);
    const children = useSelector(selectCurrentNamespaceChildren);
    const [val, setVal] = React.useState("");
    const [createOpen, setCreateOpen] = React.useState(false);
    const [createName, setCreateName] = React.useState("");
    const [createError, setCreateError] = React.useState<string | null>(null);
    const [createLoading, setCreateLoading] = React.useState(false);

    useEffect(() => {
        setVal(ns?.path || "");
    }, [ns]);

    const refreshList = () => {
        if(ns?.path) {
            dispatch(setCurrentNamespace(ns.path));
        }
    }

    const openCreateDialog = () => {
        setCreateName("");
        setCreateError(null);
        setCreateOpen(true);
    };

    const closeCreateDialog = () => {
        if (!createLoading) {
            setCreateOpen(false);
            setCreateError(null);
        }
    };

    const handleChange = (event: SelectChangeEvent) => {
        let path = event.target.value as string;

        if (path === ACTION_ADD) {
            openCreateDialog();
            return;
        }

        if (path === ACTION_REFRESH) {
            refreshList();
            return;
        }

        if (path === ACTION_NAVIGATE_ROOT) {
            path = ROOT_NAMESPACE_PATH;
        }

        if (path === ACTION_NAVIGATE_PARENT) {
            path = parentNamespace(ns?.path);
        }

        if (path.startsWith(ACTION_PREFIX)) {
            return;
        }

        dispatch(setCurrentNamespace(path));
        setVal(path);
    };

    const createNamespacePath = childNamespacePath(ns?.path, createName.trim());

    const submitCreate = async (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();

        const leafName = createName.trim();
        const validationError = validateNamespaceLeafName(leafName);
        if (validationError) {
            setCreateError(validationError);
            return;
        }

        const path = childNamespacePath(ns?.path, leafName);
        setCreateError(null);
        setCreateLoading(true);
        try {
            const res = await namespaces.create({path});
            setCreateOpen(false);
            setCreateName("");
            dispatch(setCurrentNamespace(res.data.path));
            setVal(res.data.path);
        } catch (err: any) {
            const msg = err?.response?.data?.error || err.message || 'Failed to create namespace';
            setCreateError(msg);
        } finally {
            setCreateLoading(false);
        }
    };

    let currentNamespaceItem = (
        <MenuItem value={val || ACTION_LOADING_NAMESPACE}>
            <ListItemIcon>
                <RefreshIcon />
            </ListItemIcon>
            <CircularProgress />
        </MenuItem>
    );

    if(loadingStatus === 'succeeded') {
        currentNamespaceItem = (
            <MenuItem value={ns?.path}>
                <ListItemAvatar>
                    <Avatar alt={ns?.path}>
                        {ns?.path == ROOT_NAMESPACE_PATH ?
                            <AccountTreeIcon sx={{fontSize: '1rem'}}/> :
                            <FolderIcon sx={{fontSize: '1rem'}}/>
                        }
                    </Avatar>
                </ListItemAvatar>
                <ListItemText primary={leafNamespace(ns?.path)} secondary={ns?.path}/>
            </MenuItem>
        );
    } else if(loadingStatus === 'failed') {
        currentNamespaceItem = (
            <MenuItem value="">
                <ListItemAvatar>
                    <Avatar alt="error">
                        <ErrorIcon sx={{fontSize: '1rem'}}/>
                    </Avatar>
                </ListItemAvatar>
                <ListItemText primary="Error" secondary="Error loading current namespace"/>
            </MenuItem>
        );
    }

    let childNamespaceItems = [(
        <MenuItem key={ACTION_CHILDREN} value={ACTION_CHILDREN}>
            <ListItemIcon>
                <RefreshIcon />
            </ListItemIcon>
            <CircularProgress />
        </MenuItem>
    )];

    if(childLoadingStatus === 'succeeded') {
        childNamespaceItems = children.flatMap((child, idx) => {
            const items = [
                <MenuItem key={child?.path} value={child?.path}>
                    <ListItemAvatar>
                        <Avatar alt={child?.path}>
                            <FolderIcon sx={{fontSize: '1rem'}}/>
                        </Avatar>
                    </ListItemAvatar>
                    <ListItemText primary={leafNamespace(child?.path)} secondary={child?.path}/>
                </MenuItem>
            ];

            if(idx < children.length - 1) {
                items.push(<Divider key={`divider-${idx}`} />);
            }

            return items;
        });
    } else if(childLoadingStatus === 'failed') {
        childNamespaceItems = [(
            <MenuItem key="child-error" value="" disabled={true}>
                <ListItemAvatar>
                    <Avatar alt="error">
                        <ErrorIcon sx={{fontSize: '1rem'}}/>
                    </Avatar>
                </ListItemAvatar>
                <ListItemText primary="Error" secondary="Error loading child namespace"/>
            </MenuItem>
        )];
    }

    return (
        <>
            <Select
                labelId="namespace-select"
                id="namespace-simple-select"
                value={val}
                onChange={handleChange}
                displayEmpty
                inputProps={{'aria-label': 'Select namespace'}}
                fullWidth
                sx={{
                    maxHeight: 56,
                    width: 215,
                    '&.MuiList-root': {
                        p: '8px',
                    },
                    [`& .${selectClasses.select}`]: {
                        display: 'flex',
                        alignItems: 'center',
                        gap: '2px',
                        pl: 1,
                    },
                }}
            >
                <ListSubheader sx={{pt: 0}}>Current Namespace</ListSubheader>
                {currentNamespaceItem}

                {childNamespaceItems.length == 0 ?
                    null :
                    ([
                        <ListSubheader key="children-header">Children</ListSubheader>,
                        ...childNamespaceItems
                    ])
                }

                {depth(ns?.path) > 0 ?
                    ([
                        <ListSubheader key="navigate-header" sx={{pt: 0}}>Navigate</ListSubheader>,
                        <MenuItem key={ACTION_NAVIGATE_PARENT} value={ACTION_NAVIGATE_PARENT}>
                            <ListItemIcon>
                                <ArrowBackIcon />
                            </ListItemIcon>
                            <ListItemText primary="Go to parent" secondary={`Return to ${parentNamespace(ns?.path)}`} />
                        </MenuItem>
                    ]) : []
                }

                {depth(ns?.path) > 1 ?
                    (
                        <MenuItem value={ACTION_NAVIGATE_ROOT}>
                            <ListItemIcon>
                                <AccountTreeIcon sx={{fontSize: '1rem'}}/>
                            </ListItemIcon>
                            <ListItemText primary={`Go to ${ROOT_NAMESPACE_PATH}`} />
                        </MenuItem>
                    ) : null
                }

                <ListSubheader sx={{pt: 0}}>Actions</ListSubheader>
                <MenuItem value={ACTION_REFRESH}>
                    <ListItemIcon>
                        <RefreshIcon />
                    </ListItemIcon>
                    <ListItemText primary="Refresh" secondary="Refresh namespace list" />
                </MenuItem>
                <MenuItem value={ACTION_ADD}>
                    <ListItemIcon>
                        <AddRoundedIcon />
                    </ListItemIcon>
                    <ListItemText primary="Add namespace" secondary={`Add a new namespace under ${ns?.path || 'root'}.`} />
                </MenuItem>
            </Select>
            <Dialog
                open={createOpen}
                onClose={closeCreateDialog}
                fullWidth
                maxWidth="xs"
                PaperProps={{
                    component: 'form',
                    onSubmit: submitCreate,
                }}
            >
                <DialogTitle>Add namespace</DialogTitle>
                <DialogContent>
                    {createError && <Alert severity="error" sx={{mb: 2}}>{createError}</Alert>}
                    <TextField
                        autoFocus
                        fullWidth
                        margin="dense"
                        label="Name"
                        value={createName}
                        onChange={(event) => {
                            setCreateName(event.target.value);
                            setCreateError(null);
                        }}
                        disabled={createLoading}
                        error={Boolean(validateNamespaceLeafName(createName.trim())) && createName.trim().length > 0}
                        helperText={createName.trim() ? createNamespacePath : ' '}
                    />
                </DialogContent>
                <DialogActions>
                    <Button onClick={closeCreateDialog} disabled={createLoading}>Cancel</Button>
                    <Button type="submit" variant="contained" disabled={createLoading}>
                        {createLoading ? <CircularProgress size={18} /> : 'Create'}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
}
