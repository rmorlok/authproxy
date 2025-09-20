import * as React from 'react';
import LoadingPage from "./LoadingPage";
import { BrowserRouter, Route, Routes, Outlet, Navigate } from "react-router-dom";
import { useSelector, useDispatch } from "react-redux";
import { selectAuthStatus, initiateSessionAsync } from "./store";
import { Error } from "./Error";
import Layout from "./Layout"
import HomePage from "./pages/Home";

export default function App() {
    const dispatch = useDispatch();
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
                      <Route path={'/'} element={<Navigate to="/home" replace />} />
                      <Route path={'/home'} Component={HomePage}/>
                  </Route>
              </Route>
          </Routes>
      </BrowserRouter>
  );
}
