import * as React from 'react';
import LoadingPage from "./LoadingPage";
import { BrowserRouter, Route, Routes, Outlet, Navigate } from "react-router-dom";
import { useSelector, useDispatch } from "react-redux";
import { selectAuthStatus, initiateSessionAsync } from "./store";
import { Error } from "./Error";
import type {} from '@mui/x-date-pickers-pro/themeAugmentation';
import type {} from '@mui/x-charts/themeAugmentation';
import type {} from '@mui/x-data-grid/themeAugmentation';
import type {} from '@mui/x-tree-view/themeAugmentation';
import { alpha } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import Box from '@mui/material/Box';
import Stack from '@mui/material/Stack';
import AppNavbar from './components/AppNavbar';
import Header from './components/Header';
import MainGrid from './components/MainGrid';
import SideMenu from './components/SideMenu';
import AppTheme from './shared-theme/AppTheme';
import {
    chartsCustomizations,
    dataGridCustomizations,
    datePickersCustomizations,
    treeViewCustomizations,
} from './theme/customizations';

const xThemeComponents = {
    ...chartsCustomizations,
    ...dataGridCustomizations,
    ...datePickersCustomizations,
    ...treeViewCustomizations,
};

export default function App() {
    const dispatch = useDispatch();
    const authStatus = useSelector(selectAuthStatus);

    if(authStatus === 'checking' || authStatus === 'redirecting') {
        return (
            <LoadingPage />
        );
    }

    return (
        <Dashboard />
    );
}

function Dashboard(props: { disableCustomTheme?: boolean }) {
    return (
        <AppTheme {...props} themeComponents={xThemeComponents}>
            <CssBaseline enableColorScheme />
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
                        <MainGrid />
                    </Stack>
                </Box>
            </Box>
        </AppTheme>
    );
}

// const ProtectedRoutes = () => {
//     const authStatus = useSelector(selectAuthStatus);
//
//     // Don't redirect on loading states
//     return authStatus !== 'unauthenticated' ? (
//         <Outlet />
//     ) : (
//         <Error title={"Unauthorized"} body1={"You are not authorized to access this page. Please login to continue."} body2={"If you are not already logged in, please click the button below to login."} />
//     );
// };


// export function Router() {
//   return (
//       <BrowserRouter>
//           <Routes>
//               { /* Things only authenticated users can see */ }
//               <Route element={<ProtectedRoutes />}>
//                   <Route element={<Layout />}>
//                       <Route path={'/'} element={<Navigate to="/connections" replace />} />
//                       <Route path={'/connectors'} Component={ConnectorList}/>
//                       <Route path={'/connections'} Component={ConnectionList}/>
//                   </Route>
//               </Route>
//           </Routes>
//       </BrowserRouter>
//   );
// }
