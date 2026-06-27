import Box from "@mui/material/Box";
import SideMenu from "./components/SideMenu";
import AppNavbar from "./components/AppNavbar";
import {alpha} from "@mui/material/styles";
import Stack from "@mui/material/Stack";
import Header from "./components/Header";
import * as React from "react";
import { Outlet } from 'react-router-dom';
import type {} from '@mui/x-date-pickers-pro/themeAugmentation';
import type {} from '@mui/x-charts/themeAugmentation';
import type {} from '@mui/x-data-grid/themeAugmentation';
import type {} from '@mui/x-tree-view/themeAugmentation';
import Copyright from "./components/Copyright";
import {ROOT_NAMESPACE_PATH} from "@authproxy/api";
import {useDispatch, useSelector} from "react-redux";
import {AppDispatch} from "./store";
import {parseAsString, useQueryState} from "nuqs";
import {
    isValidNamespacePath,
    normalizeNamespacePath,
    selectCurrentNamespacePath, selectHasInitializedNamespace,
    setCurrentNamespace
} from "./store/namespacesSlice";
import {useEffect} from "react";
import CircularProgress from "@mui/material/CircularProgress";

const NS_LOCALSTORAGE_KEY = 'ns';
const DEFAULT_NAMESPACE_PATH_QUERY_SENTINEL = ROOT_NAMESPACE_PATH;

export default function Layout(_props: { disableCustomTheme?: boolean }) {
    const dispatch = useDispatch<AppDispatch>();
    const [queryNs, setQueryNs] = useQueryState<string>('ns', parseAsString.withDefault(DEFAULT_NAMESPACE_PATH_QUERY_SENTINEL));

    const nsPath = useSelector(selectCurrentNamespacePath);
    const hasInitializedNs = useSelector(selectHasInitializedNamespace);

    useEffect(() => {
        if(!hasInitializedNs) {
            const storedPath = typeof window !== 'undefined' ?
                localStorage.getItem(NS_LOCALSTORAGE_KEY) :
                null;

            let targetPath = normalizeNamespacePath(storedPath);

            if (queryNs !== DEFAULT_NAMESPACE_PATH_QUERY_SENTINEL && isValidNamespacePath(queryNs)) {
                targetPath = queryNs;
            }

            // Start loading the information for the namespace
            dispatch(setCurrentNamespace(targetPath));

            if (queryNs !== DEFAULT_NAMESPACE_PATH_QUERY_SENTINEL && queryNs !== targetPath) {
                void setQueryNs(targetPath);
            }
        } else {
            const currentPath = normalizeNamespacePath(nsPath);

            if (currentPath !== nsPath) {
                dispatch(setCurrentNamespace(currentPath));
                return;
            }

            if(currentPath === ROOT_NAMESPACE_PATH) {
                void setQueryNs(DEFAULT_NAMESPACE_PATH_QUERY_SENTINEL);
            } else {
                void setQueryNs(currentPath);
            }

            if (typeof window !== 'undefined') {
                localStorage.setItem(NS_LOCALSTORAGE_KEY, currentPath);
            }
        }
    }, [hasInitializedNs, nsPath, queryNs, dispatch, setQueryNs])

    if(!hasInitializedNs) {
        return (
            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
                <CircularProgress />
            </Box>
        );
    }

    return (
        <>
            <Box sx={{ display: 'flex' }}>
                <SideMenu />
                <AppNavbar />
                {/* Main content */}
                <Box
                    component="main"
                    sx={(theme) => ({
                        flexGrow: 1,
                        backgroundColor: theme.vars
                            ? `rgba(${theme.vars.palette.background.defaultChannel} / 1)`
                            : alpha(theme.palette.background.default, 1),
                        overflow: 'auto',
                    })}
                >
                    <Stack
                        spacing={2}
                        sx={{
                            alignItems: 'center',
                            mx: 3,
                            pb: 5,
                            mt: { xs: 8, md: 0 },
                        }}
                    >
                        <Header />
                        <Outlet />
                        <Copyright sx={{ my: 4 }} />
                    </Stack>
                </Box>
            </Box>
        </>
    );
}
