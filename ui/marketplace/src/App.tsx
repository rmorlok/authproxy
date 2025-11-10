import * as React from 'react';
import LoadingPage from "./LoadingPage";
import { BrowserRouter, Route, Routes, Outlet, Navigate } from "react-router-dom";
import { useSelector } from "react-redux";
import { selectAuthStatus } from "./store";
import Layout from './components/Layout';
import ConnectorList from './components/ConnectorList';
import ConnectionList from './components/ConnectionList';
import { Error } from "./Error";

export default function App() {
    const authStatus = useSelector(selectAuthStatus);

    if(authStatus === 'checking' || authStatus === 'redirecting') {
        return (
            <LoadingPage />
        );
    }

    return (
        <Router />
    );
}

const ProtectedRoutes = () => {
    const authStatus = useSelector(selectAuthStatus);

    // Don't redirect on loading states
    return authStatus !== 'unauthenticated' ? (
        <Outlet />
    ) : (
        <Error title={"Unauthorized"} body1={"You are not authorized to access this page. Please login to continue."} body2={"If you are not already logged in, please click the button below to login."} />
    );
};

export function Router() {
  return (
      <BrowserRouter>
          <Routes>
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
