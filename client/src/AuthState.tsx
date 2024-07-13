import { useSelector, useDispatch } from 'react-redux'
import * as React from "react";
import {
    selectAuthStatus,
    selectUser,
    loginWithProvider,
    logout,
    AppDispatch,
} from './store'
import Box from "@mui/material/Box";
import {Button} from "@mui/material";

// This is a dummy component for testing
export function AuthState(props: any) {
    const status = useSelector(selectAuthStatus);
    const user = useSelector(selectUser);
    const dispatch = useDispatch<AppDispatch>();

    if (status === 'checking') {
        return (<Box>Checking...</Box>);
    } else if (status === 'authenticated' && user != null) {
        return (<Box>{user.name} is authenticated. <Button variant="outlined" onClick={() => dispatch(logout())}>Logout</Button></Box>);
    } else {
        return (<Box>Unauthenticated. <Button variant="outlined" onClick={() => dispatch(loginWithProvider('google'))}>Login</Button></Box>);
    }
}