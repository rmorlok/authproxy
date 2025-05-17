import * as React from 'react';
import { useEffect } from 'react';
import LoadingPage from "./LoadingPage";
import { BrowserRouter, Route, Routes, Outlet, Navigate } from "react-router-dom";
import SignIn from "./SignIn";
import { useSelector, useDispatch } from "react-redux";
import { selectAuthStatus, loadAuthStateAsync, loadProvidersAsync } from "./store";
import Layout from './components/Layout';
import ConnectorList from './components/ConnectorList';
import ConnectionList from './components/ConnectionList';

export default function App() {
    const dispatch = useDispatch();
    const authStatus = useSelector(selectAuthStatus);

    useEffect(() => {
        // Load auth state and providers when the app starts
        dispatch(loadAuthStateAsync());
        dispatch(loadProvidersAsync());
    }, [dispatch]);

    if(authStatus === 'checking' || authStatus === 'redirecting') {
        return (
            <LoadingPage />
        );
    }

    return (
        <Router />
    );
}

const GuestRoute = () => {
    const authStatus = useSelector(selectAuthStatus);

    return authStatus === 'unauthenticated' ? (
        <Outlet />
    ) : (
        <Navigate to="/" replace />
    );
};

const ProtectedRoutes = () => {
    const authStatus = useSelector(selectAuthStatus);

    // Don't redirect on loading states
    return authStatus !== 'unauthenticated' ? (
        <Outlet />
    ) : (
        <Navigate to="/login" replace />
    );
};

export function Router() {
  return (
      <BrowserRouter>
          <Routes>
              { /* Things people can see unauthenticated */ }
              <Route element={<GuestRoute />}>
                  <Route path={'/login'} Component={SignIn}/>
              </Route>

              { /* Things only authenticated users can see */ }
              <Route element={<ProtectedRoutes />}>
                  <Route element={<Layout />}>
                      <Route path={'/'} element={<Navigate to="/connections" replace />} />
                      <Route path={'/connectors'} Component={ConnectorList}/>
                      <Route path={'/connections'} Component={ConnectionList}/>
                  </Route>
              </Route>
          </Routes>
      </BrowserRouter>
  );
}
