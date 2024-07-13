import * as React from 'react';
import LoadingPage from "./LoadingPage";
import {BrowserRouter, Route, Routes, Outlet, Navigate} from "react-router-dom";
import SignIn from "./SignIn";
import GameBoard from "./GameBoard";
import ChooseTable from "./ChooseTable";
import {AuthState} from "./AuthState";
import {useSelector} from "react-redux";
import {selectAuthStatus} from "./store";

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
                  <Route path={'/'} Component={ChooseTable}/>
                  <Route path={'/table/:id'} Component={GameBoard}/>
              </Route>
          </Routes>
      </BrowserRouter>
  );
}
