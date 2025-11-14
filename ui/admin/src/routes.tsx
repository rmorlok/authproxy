import {createBrowserRouter, Navigate, Params} from 'react-router-dom';
import Layout from "./Layout";
import ListParent from "./ListParent";
import HomePage from "./pages/Home";
import ConnectorsPage from "./pages/Connectors";
import ConnectionsPage from "./pages/Connections";
import RequestsPage from "./pages/Requests";
import RequestDetail from "./pages/RequestDetail";
import ConnectorDetail from "./pages/ConnectorDetail";
import ConnectionDetail from "./pages/ConnectionDetail";
import ConnectorVersionDetail from "./pages/ConnectorVersionDetail";
import ActorsPage from "./pages/Actors";
import TasksPage from "./pages/Tasks";
import AboutPage from "./pages/About";
import * as React from "react";


export const router = createBrowserRouter([
    {
        path: '/',
        element: <Layout />,
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
                element: (<ListParent><ConnectorsPage /></ListParent>),
                handle: { title: 'Connectors' },
            },
            {
                path: 'connectors/:id',
                element: <ConnectorDetail />,
                handle: [
                    {
                        title: 'Connectors',
                        path: (_params: Params<string>) => `/connectors`,
                    },
                    {
                        attr: 'id',
                        path: (params: Params<string>) => `/connectors/${params.id}`
                    },
                ],
            },
            {
                path: 'connectors/:id/versions/:version',
                element: <ConnectorVersionDetail />,
                handle: [
                    {
                        title: 'Connectors',
                        path: (_params: Params<string>) => `/connectors`,
                    },
                    {
                        attr: 'id',
                        path: (params: Params<string>) => `/connectors/${params.id}`,
                    },
                    {
                        title: 'Versions',
                        path: (params: Params<string>) => `/connectors/${params.id}`,
                    },
                    {
                        attr: 'version',
                        path: (params: Params<string>) => `/connectors/${params.id}/versions/${params.version}`,
                    }
                ],
            },
            {
                path: 'connections',
                element: (<ListParent><ConnectionsPage /></ListParent>),
                handle: { title: 'Connections' },
                children: [
                    {
                        path: ':id',
                        element: <ConnectionDetail />,
                    }
                ]
            },
            {
                path: 'connections/:id',
                element: <ConnectionDetail />,
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
