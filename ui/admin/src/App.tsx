import * as React from 'react';
import LoadingPage from "./LoadingPage";
import { NuqsAdapter } from 'nuqs/adapters/react'
import { RouterProvider } from "react-router-dom";
import { useSelector } from "react-redux";
import { selectAuthStatus } from "./store";
import { router } from './routes';

export default function App() {
    const authStatus = useSelector(selectAuthStatus);

    if(authStatus === 'checking' || authStatus === 'redirecting') {
        return (
            <LoadingPage />
        );
    }

    return (
        <NuqsAdapter>
            <RouterProvider router={router} />
        </NuqsAdapter>
    );
}