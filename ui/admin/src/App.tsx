import * as React from 'react';
import LoadingPage from "./LoadingPage";
import { NuqsAdapter } from 'nuqs/adapters/react'
import { RouterProvider } from "react-router-dom";
import { useSelector } from "react-redux";
import { selectAuthStatus } from "./store";
import { router } from './routes';
import Dev from './pages/Dev';

export default function App() {
    const authStatus = useSelector(selectAuthStatus);

    if (import.meta.env.DEV && window.location.pathname === '/dev') {
        return <Dev />;
    }

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