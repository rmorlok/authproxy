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
import {NAMESPACE_PATH_SEPARATOR, ROOT_NAMESPACE_PATH} from "@authproxy/api";

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

export default function NamespaceSelector() {
    const dispatch = useDispatch<AppDispatch>();
    const loadingStatus = useSelector(selectNamespaceStatus);
    const childLoadingStatus = useSelector(selectNamespaceChildrenStatus);
    const ns = useSelector(selectCurrentNamespace);
    const children = useSelector(selectCurrentNamespaceChildren);
    const [val, setVal] = React.useState("");

    useEffect(() => {
        setVal(ns?.path || "");
    }, [ns]);

    const handleChange = (event: SelectChangeEvent) => {
        let path = event.target.value as string;

        if (path === "...action.navigate-root") {
            path = ROOT_NAMESPACE_PATH;
        }

        if (path === "...action.navigate-parent") {
            path = parentNamespace(ns?.path);
        }

        if (path.startsWith("...action.")) {
            return;
        }

        dispatch(setCurrentNamespace(path));
        setVal(path);
    };

    const refreshList = () => {
        if(ns?.path) {
            dispatch(setCurrentNamespace(ns.path));
        }
    }

    let currentNamespaceItem = (
        <MenuItem value="...action.loading-ns">
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
        <MenuItem key="...action.children" value="...action.children">
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
                    <MenuItem key="...action.navigate-parent" value="...action.navigate-parent">
                        <ListItemIcon>
                            <ArrowBackIcon />
                        </ListItemIcon>
                        <ListItemText primary="Go to parent" secondary={`Return to ${parentNamespace(ns?.path)}`} />
                    </MenuItem>
                ]) : []
            }

            {depth(ns?.path) > 1 ?
                (
                    <MenuItem value="...action.navigate-root">
                        <ListItemIcon>
                            <AccountTreeIcon sx={{fontSize: '1rem'}}/>
                        </ListItemIcon>
                        <ListItemText primary={`Go to ${ROOT_NAMESPACE_PATH}`} />
                    </MenuItem>
                ) : null
            }

            <ListSubheader sx={{pt: 0}}>Actions</ListSubheader>
            <MenuItem value="///action/refresh" onClick={refreshList}>
                <ListItemIcon>
                    <RefreshIcon />
                </ListItemIcon>
                <ListItemText primary="Refresh" secondary="Refresh namespace list" />
            </MenuItem>
            <MenuItem value="...action.add">
                <ListItemIcon>
                    <AddRoundedIcon />
                </ListItemIcon>
                <ListItemText primary="Add namespace" secondary={`Add a new namespace under ${ns?.path || 'root'}.`} />
            </MenuItem>
        </Select>
    );
}
