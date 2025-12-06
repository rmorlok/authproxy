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
import ErrorIcon from '@mui/icons-material/Error';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import FolderIcon from '@mui/icons-material/Folder';
import ConstructionRoundedIcon from '@mui/icons-material/ConstructionRounded';
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

function leafNamespace(path: string | null | undefined): string {
    if (!path) {
        return "";
    }

    const parts = path.split('/');
    if (parts.length > 1) {
        return parts[length-1];
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
        const path = event.target.value as string;
        if (path.startsWith("///action/")) {
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

    var currentNamespaceItem = (
        <MenuItem value="///action/loading-ns">
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
                        <AccountTreeIcon sx={{fontSize: '1rem'}}/>
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

    var childNamespaceItems = [(
        <MenuItem value="///action/children">
            <ListItemIcon>
                <RefreshIcon />
            </ListItemIcon>
            <CircularProgress />
        </MenuItem>
    )];

    if(childLoadingStatus === 'succeeded') {
        childNamespaceItems = children.flatMap((child, idx) => {
            var items = [
                <MenuItem value={child?.path}>
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
                <MenuItem value="" disabled={true}>
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
                    <ListSubheader>Children</ListSubheader>,
                    ...childNamespaceItems
                ])
            }

            <MenuItem value="///action/refresh" onClick={refreshList}>
                <ListItemIcon>
                    <RefreshIcon />
                </ListItemIcon>
                <ListItemText primary="Refresh" secondary="Refresh namespace list" />
            </MenuItem>

            <MenuItem value="///action/add">
                <ListItemIcon>
                    <AddRoundedIcon />
                </ListItemIcon>
                <ListItemText primary="Add namespace" secondary={`Add a new namespace under ${ns?.path || 'root'}.`} />
            </MenuItem>
        </Select>
    );
}
