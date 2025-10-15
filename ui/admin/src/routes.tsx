import {createBrowserRouter, Navigate} from 'react-router-dom';
import Layout from "./Layout";
import ListParent from "./ListParent";
import HomePage from "./pages/Home";
import ConnectorsPage from "./pages/Connectors";
import ConnectionsPage from "./pages/Connections";
import RequestsPage from "./pages/Requests";
import RequestDetail from "./pages/RequestDetail";
import ActorsPage from "./pages/Actors";
import TasksPage from "./pages/Tasks";
import AboutPage from "./pages/About";
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
                element: (<ListParent><RequestsPage /></ListParent>),
                handle: { title: 'Requests' },
                children: [
                    {
                        path: ':id',
                        element: <RequestDetail />,
                    }
                ]
            },
            {
                path: 'requests/:id',
                element: <RequestDetail />,
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
            {
                path: 'about',
                element: <AboutPage />,
                handle: { title: 'About' }
            },
        ]
    }
]);
