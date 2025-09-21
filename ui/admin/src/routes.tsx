import {BrowserRouter, createBrowserRouter, Navigate, Route, RouterProvider, Routes} from 'react-router-dom';
import Layout from "./Layout";
import HomePage from "./pages/Home";
import ConnectorsPage from "./pages/Connectors";
import ConnectionsPage from "./pages/Connections";
import RequestsPage from "./pages/Requests";
import ActorsPage from "./pages/Actors";
import TasksPage from "./pages/Tasks";
import * as React from "react";

export const router = createBrowserRouter([
    {
        path: '/',
        element: <Layout disableCustomTheme={true} />,
        handle: { title: 'AuthProxy Admin' },
        children: [
            {
                path: '',
                element: <Navigate to="/home" replace />,
            },
            {
                path: 'home',
                element: <HomePage />,
                handle: { title: 'Home' }
            },
            {
                path: 'connectors',
                element: <ConnectorsPage />,
                handle: { title: 'Connectors' }
            },
            {
                path: 'connections',
                element: <ConnectionsPage />,
                handle: { title: 'Connections' }
            },
            {
                path: 'requests',
                element: <RequestsPage />,
                handle: { title: 'Requests' }
            },
            {
                path: 'actors',
                element: <ActorsPage />,
                handle: { title: 'Actors' }
            },
            {
                path: 'tasks',
                element: <TasksPage />,
                handle: { title: 'Tasks' }
            },
        ]
    }
]);
