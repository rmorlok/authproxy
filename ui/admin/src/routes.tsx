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
import KeysPage from "./pages/Keys";
import KeyDetail from "./pages/KeyDetail";
import RateLimitsPage from "./pages/RateLimits";
import RateLimitDetail from "./pages/RateLimitDetail";
import RateLimitDryRunPage from "./pages/RateLimitDryRun";
import TasksPage from "./pages/Tasks";
import TaskQueueDetailPage from "./pages/TaskQueueDetail";
import WorkflowsPage from "./pages/Workflows";
import WorkflowDetailPage from "./pages/WorkflowDetail";
import NamespaceDetailPage, {CurrentNamespaceDetailRedirect} from "./pages/NamespaceDetail";
import NamespacesPage from "./pages/Namespaces";
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
                path: 'keys',
                element: (<ListParent><KeysPage /></ListParent>),
                handle: { title: 'Keys' },
            },
            {
                path: 'keys/:id',
                element: <KeyDetail />,
                handle: [
                    {
                        title: 'Keys',
                        path: (_params: Params<string>) => `/keys`,
                    },
                    {
                        attr: 'id',
                        path: (params: Params<string>) => `/keys/${params.id}`,
                    },
                ],
            },
            {
                path: 'rate-limits',
                element: (<ListParent><RateLimitsPage /></ListParent>),
                handle: { title: 'Rate Limits' },
            },
            {
                path: 'rate-limits/_dryRun',
                element: <RateLimitDryRunPage />,
                handle: [
                    {
                        title: 'Rate Limits',
                        path: (_params: Params<string>) => `/rate-limits`,
                    },
                    { title: 'Dry run' },
                ],
            },
            {
                path: 'rate-limits/:id',
                element: <RateLimitDetail />,
                handle: [
                    {
                        title: 'Rate Limits',
                        path: (_params: Params<string>) => `/rate-limits`,
                    },
                    {
                        attr: 'id',
                        path: (params: Params<string>) => `/rate-limits/${params.id}`,
                    },
                ],
            },
            {
                path: 'namespaces',
                element: (<ListParent><NamespacesPage /></ListParent>),
                handle: {title: 'Namespaces'},
            },
            {
                path: 'namespaces/:path',
                element: <NamespaceDetailPage />,
                handle: [
                    {
                        title: 'Namespaces',
                        path: (_params: Params<string>) => '/namespaces',
                    },
                    {
                        attr: 'path',
                        path: (params: Params<string>) => `/namespaces/${encodeURIComponent(params.path || '')}`,
                    },
                ],
            },
            {
                path: 'namespace',
                element: <CurrentNamespaceDetailRedirect />,
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
                path: 'internal-tasks',
                element: <TasksPage />,
                handle: { title: 'Internal Tasks' }
            },
            {
                path: 'internal-tasks/queues/:queue',
                element: <TaskQueueDetailPage />,
                handle: [
                    {
                        title: 'Internal Tasks',
                        path: (_params: Params<string>) => `/internal-tasks`,
                    },
                    {
                        title: 'Queues',
                        path: (_params: Params<string>) => `/internal-tasks`,
                    },
                    {
                        attr: 'queue',
                        path: (params: Params<string>) => `/internal-tasks/queues/${params.queue}`,
                    },
                ],
            },
            {
                path: 'internal-workflows',
                element: <WorkflowsPage />,
                handle: { title: 'Internal Workflows' }
            },
            {
                path: 'internal-workflows/:instanceId/:executionId',
                element: <WorkflowDetailPage />,
                handle: [
                    {
                        title: 'Internal Workflows',
                        path: (_params: Params<string>) => `/internal-workflows`,
                    },
                    {
                        attr: 'instanceId',
                        path: (params: Params<string>) => `/internal-workflows/${params.instanceId}/${params.executionId}`,
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
