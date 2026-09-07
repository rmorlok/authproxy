import * as React from 'react';
import { Meta, StoryObj } from '@storybook/react';
import { Provider } from 'react-redux';
import { combineReducers, configureStore } from '@reduxjs/toolkit';
import { Route, Routes } from 'react-router-dom';
import { ThemeProvider } from '@emotion/react';
import { CssBaseline } from '@mui/material';
import {
  Connection,
  ConnectionHealthState,
  ConnectionState,
  Connector,
} from '@authproxy/api';
import theme from '../theme';
import Layout from '../components/Layout';
import ConnectorDetail from '../components/ConnectorDetail';
import ConnectorList from '../components/ConnectorList';
import ConnectionList from '../components/ConnectionList';
import authReducer from '../store/sessionSlice';
import connectorsReducer from '../store/connectorsSlice';
import connectionsReducer from '../store/connectionsSlice';
import notificationsReducer from '../store/notificationsSlice';
import toastsReducer from '../store/toastsSlice';
import { connectionFixture, connectorFixture, notificationFixture } from '../testing/resources';

const logoDataUri = (label: string, background: string, foreground = '#ffffff') => {
  const initials = label
    .split(/\s+/)
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${label} logo"><rect width="280" height="140" rx="8" fill="${background}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${foreground}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${initials}</text></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
};

const connectors: Connector[] = [
  connectorFixture({
    id: 'google-drive',
    displayName: 'Google Drive',
    description: 'Have the agent track your work in Google Drive.',
    highlight: 'Have the agent track your work in Google Drive.',
    logo: {publicUrl: logoDataUri('Google Drive', '#188038')},
    hasConfigure: false,
  }),
  connectorFixture({
    id: 'greenhouse',
    displayName: 'Greenhouse',
    description: 'This integration pushes candidates to greenhouse.',
    highlight: 'This integration pushes candidates to greenhouse.',
    logo: {publicUrl: logoDataUri('Greenhouse', '#24a47f')},
    hasConfigure: false,
  }),
  connectorFixture({
    id: 'google-calendar',
    displayName: 'Google Calendar',
    description: `Google Calendar lets agents coordinate scheduling work without needing direct access to your primary app.

![Calendar workflow preview](/calendar-workflow-preview.svg)

### What agents can do

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create and update events | Yes |
| Read attendee responses | Yes |
| Manage private event details | No |

Use this connector when the assistant should propose meeting times, create holds, or keep follow-up work attached to calendar events.`,
    highlight: 'Coordinate meetings, availability, and follow-up from Google Calendar.',
    logo: {publicUrl: logoDataUri('Google Calendar', '#1a73e8')},
    hasConfigure: true,
  }),
  connectorFixture({
    id: 'gmail',
    displayName: 'GMail',
    description: 'Have the agent respond to your emails without you needing to be involved. Like magic.',
    highlight: 'Have the agent respond to your emails without you needing to be involved. Like magic.',
    logo: {publicUrl: logoDataUri('GMail', '#d93025')},
    hasConfigure: false,
  }),
  connectorFixture({
    id: 'pipedrive',
    displayName: 'pipedrive',
    description: 'Allow our agent to handle your sales support.',
    highlight: 'Allow our agent to handle your sales support.',
    logo: {publicUrl: logoDataUri('pipedrive', '#017a5e')},
    hasConfigure: false,
  }),
  connectorFixture({
    id: 'asana',
    displayName: 'Asana',
    description: 'Allow our agent organize your work.',
    highlight: 'Allow our agent organize your work.',
    logo: {publicUrl: logoDataUri('Asana', '#f06a6a')},
    hasConfigure: false,
  }),
];

interface ConnectionOverrides {
  state?: ConnectionState;
  healthState?: ConnectionHealthState;
  setupStepId?: string;
}

const connectionFor = (
  connector: Connector,
  overrides: ConnectionOverrides = {},
): Connection => connectionFixture({
  id: `cxn_${connector.metadata.id}`,
  connector,
  ...overrides,
});

const populatedConnections: Connection[] = [
  connectionFor(connectors[0]),
  connectionFor(connectors[2], { healthState: ConnectionHealthState.UNHEALTHY }),
  connectionFor(connectors[5], { state: ConnectionState.SETUP }),
  connectionFor(connectors[4], { state: ConnectionState.DISABLED }),
];

const setupStep = {
  connectionId: 'cxn_google-calendar',
  stepId: 'select-calendar',
  stepTitle: 'Select a Calendar',
  stepDescription: 'Choose which Google Calendar the agent should manage.',
  currentStep: 0,
  totalSteps: 2,
  jsonSchema: {
    type: 'object',
    required: ['calendar_id'],
    properties: {
      calendar_id: {
        type: 'string',
        title: 'Calendar',
        enum: ['primary', 'product', 'support'],
      },
    },
  },
  uiSchema: {
    type: 'VerticalLayout',
    elements: [{ type: 'Control', scope: '#/properties/calendar_id' }],
  },
};

const baseConnectionsState = {
  items: populatedConnections,
  status: 'succeeded',
  error: null,
  initiatingConnection: false,
  initiationError: null,
  disconnectingConnection: false,
  disconnectionError: null,
  currentTaskId: null,
  currentFormStep: null,
  submittingForm: false,
  formSubmitError: null,
  verifyingConnectionId: null,
  verifyError: null,
  retryingConnection: false,
  recentlyCompletedConnectionId: null,
};

const baseNotificationsState = {
  items: [],
  status: 'succeeded',
  error: null,
  markingViewed: false,
};

function MarketplaceStory({
  route,
  connectorsState = { items: connectors, status: 'succeeded', error: null },
  connectionsState = baseConnectionsState,
  notificationsState = baseNotificationsState,
}: {
  route: '/connectors' | '/connector-detail' | '/connections';
  connectorsState?: Record<string, unknown>;
  connectionsState?: Record<string, unknown>;
  notificationsState?: Record<string, unknown>;
}) {
  const store = configureStore({
    reducer: combineReducers({
      auth: authReducer,
      connectors: connectorsReducer,
      connections: connectionsReducer,
      notifications: notificationsReducer,
      toasts: toastsReducer,
    }),
    preloadedState: {
      auth: { actorId: 'actor_storybook', status: 'authenticated' },
      connectors: connectorsState,
      connections: connectionsState,
      notifications: notificationsState,
      toasts: { items: [] },
    },
  });

  return (
    <Provider store={store}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <Routes>
          <Route element={<Layout />}>
            <Route
              path="*"
              element={
                route === '/connectors'
                  ? <ConnectorList />
                  : route === '/connector-detail'
                    ? <ConnectorDetail connectorId="google-calendar" />
                    : <ConnectionList />
              }
            />
          </Route>
        </Routes>
      </ThemeProvider>
    </Provider>
  );
}

const meta: Meta<typeof MarketplaceStory> = {
  title: 'Pages/Marketplace',
  component: MarketplaceStory,
  parameters: {
    layout: 'fullscreen',
  },
};

export default meta;
type Story = StoryObj<typeof MarketplaceStory>;

const mobileViewport = {
  viewport: {
    defaultViewport: 'marketplaceMobile',
  },
};

const tabletViewport = {
  viewport: {
    defaultViewport: 'marketplaceTablet',
  },
};

export const AvailableConnectors: Story = {
  args: {
    route: '/connectors',
  },
};

export const ConnectorOverview: Story = {
  args: {
    route: '/connector-detail',
  },
};

export const ConnectorOverviewMobile: Story = {
  args: {
    route: '/connector-detail',
  },
  parameters: mobileViewport,
};

export const AvailableConnectorsLoading: Story = {
  args: {
    route: '/connectors',
    connectorsState: { items: [], status: 'loading', error: null },
  },
};

export const ConnectionsPopulated: Story = {
  args: {
    route: '/connections',
  },
};

export const ConnectionsPopulatedMobile: Story = {
  args: {
    route: '/connections',
  },
  parameters: mobileViewport,
};

export const ConnectionsPopulatedTablet: Story = {
  args: {
    route: '/connections',
  },
  parameters: tabletViewport,
};

export const ConnectionsNeedsAttention: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [
        connectionFor(connectors[2], {
          healthState: ConnectionHealthState.UNHEALTHY,
        }),
      ],
    },
  },
};

export const ConnectionsWithNotifications: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [
        connectionFor(connectors[2], {
          healthState: ConnectionHealthState.UNHEALTHY,
        }),
        connectionFor(connectors[5], {
          setupStepId: 'select-workspace',
        }),
      ],
    },
    notificationsState: {
      ...baseNotificationsState,
      items: [
        notificationFixture({
          id: 'ntf_reauth',
          actionUrl: '/connections/cxn_google-calendar?action=reauth',
        }),
      ],
    },
  },
};

export const ConnectionsHealthyActions: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [
        connectionFor(connectors[2]),
        connectionFor(connectors[0]),
      ],
    },
  },
};

export const ConnectionsEmpty: Story = {
  args: {
    route: '/connections',
    connectionsState: { ...baseConnectionsState, items: [] },
  },
};

export const ConnectionsEmptyMobile: Story = {
  args: {
    route: '/connections',
    connectionsState: { ...baseConnectionsState, items: [] },
  },
  parameters: mobileViewport,
};

export const ConnectionsEmptyTablet: Story = {
  args: {
    route: '/connections',
    connectionsState: { ...baseConnectionsState, items: [] },
  },
  parameters: tabletViewport,
};

export const ConnectionsEmptyLoadingConnectors: Story = {
  args: {
    route: '/connections',
    connectorsState: { items: [], status: 'loading', error: null },
    connectionsState: { ...baseConnectionsState, items: [] },
  },
};

export const AvailableConnectorsMobile: Story = {
  args: {
    route: '/connectors',
  },
  parameters: mobileViewport,
};

export const AvailableConnectorsTablet: Story = {
  args: {
    route: '/connectors',
  },
  parameters: tabletViewport,
};

export const ConnectionSetupDialog: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
    },
  },
};

export const ConnectionSetupDialogTablet: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
    },
  },
  parameters: tabletViewport,
};

export const ConnectionSetupSubmitting: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true,
    },
  },
};

export const VerifyingConnectionDialog: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar',
    },
  },
};

export const VerificationFailedDialog: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true,
      },
    },
  },
};

export const VerificationFailedDialogTablet: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true,
      },
    },
  },
  parameters: tabletViewport,
};

export const VerificationRetryingDialog: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true,
      },
      retryingConnection: true,
    },
  },
};

export const VerificationFailedNoRetryDialog: Story = {
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'The provider rejected this setup and it cannot be retried.',
        canRetry: false,
      },
    },
  },
};
