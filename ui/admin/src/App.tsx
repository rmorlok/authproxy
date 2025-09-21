import * as React from 'react';
import LoadingPage from "./LoadingPage";
import { RouterProvider } from "react-router-dom";
import { useSelector, useDispatch } from "react-redux";
import { selectAuthStatus } from "./store";
import { router } from './routes';

export default function App() {
    const dispatch = useDispatch();
    const authStatus = useSelector(selectAuthStatus);

    if(authStatus === 'checking' || authStatus === 'redirecting') {
        return (
            <LoadingPage />
        );
    }

    return (
        <RouterProvider router={router} />
    );
}