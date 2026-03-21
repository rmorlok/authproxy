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
import ActorDetailPage from "./pages/ActorDetail";
import EncryptionKeysPage from "./pages/EncryptionKeys";
import EncryptionKeyDetail from "./pages/EncryptionKeyDetail";
import TasksPage from "./pages/Tasks";
import TaskQueueDetailPage from "./pages/TaskQueueDetail";
import NamespaceDetailPage from "./pages/NamespaceDetail";
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
                path: 'encryption-keys',
                element: (<ListParent><EncryptionKeysPage /></ListParent>),
                handle: { title: 'Encryption Keys' },
            },
            {
                path: 'encryption-keys/:id',
                element: <EncryptionKeyDetail />,
                handle: [
                    {
                        title: 'Encryption Keys',
                        path: (_params: Params<string>) => `/encryption-keys`,
                    },
                    {
                        attr: 'id',
                        path: (params: Params<string>) => `/encryption-keys/${params.id}`,
                    },
                ],
            },
            {
                path: 'namespace',
                element: <NamespaceDetailPage />,
                handle: { title: 'Namespace' }
            },
            {
                path: 'actors',
                element: <ActorsPage />,
                handle: { title: 'Actors' }
            },
            {
                path: 'actors/:id',
                element: <ActorDetailPage />,
                handle: [
                    {
                        title: 'Actors',
                        path: (_params: Params<string>) => `/actors`,
                    },
                    {
                        attr: 'id',
                        path: (params: Params<string>) => `/actors/${params.id}`,
                    },
                ],
            },
            {
                path: 'tasks',
                element: <TasksPage />,
                handle: { title: 'Tasks' }
            },
            {
                path: 'tasks/queues/:queue',
                element: <TaskQueueDetailPage />,
                handle: [
                    {
                        title: 'Tasks',
                        path: (_params: Params<string>) => `/tasks`,
                    },
                    {
                        title: 'Queues',
                        path: (_params: Params<string>) => `/tasks`,
                    },
                    {
                        attr: 'queue',
                        path: (params: Params<string>) => `/tasks/queues/${params.queue}`,
                    },
                ],
            },
            {
                path: 'about',
                element: <AboutPage />,
                handle: { title: 'About' }
            },
        ]
    }
]);
