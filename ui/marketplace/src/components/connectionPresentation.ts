import { Connection, ConnectionHealthState, ConnectionState } from '@authproxy/api';

export interface ConnectionStatusPresentation {
  createdDate: string;
  isHealthyConfigured: boolean;
  isUnhealthy: boolean;
  requiresSetup: boolean;
  requiresReconnection: boolean;
  statusBadgeLabel: string | null;
  statusBadgeColor: 'warning' | 'error';
  statusDotColor: string;
  statusText: string;
}

export const getConnectionStatusPresentation = (connection: Connection): ConnectionStatusPresentation => {
  const createdDate = new Date(connection.created_at).toLocaleDateString();
  const isUnhealthy =
    connection.state === ConnectionState.CONFIGURED &&
    connection.health_state === ConnectionHealthState.UNHEALTHY;
  const isHealthyConfigured =
    connection.state === ConnectionState.CONFIGURED &&
    !isUnhealthy;
  const requiresSetup = connection.state === ConnectionState.SETUP;
  const requiresReconnection = isUnhealthy || connection.state === ConnectionState.DISABLED;
  const statusBadgeLabel = requiresSetup
    ? 'Requires setup'
    : requiresReconnection
      ? 'Requires reconnection'
      : null;
  const statusBadgeColor: 'warning' | 'error' = requiresReconnection ? 'error' : 'warning';
  const statusText = requiresSetup
    ? 'Setup required'
    : requiresReconnection
      ? 'Reconnection required'
      : connection.state === ConnectionState.DISCONNECTING
        ? 'Disconnecting'
        : connection.state === ConnectionState.DISCONNECTED
          ? 'Disconnected'
          : `Connected on ${createdDate}`;
  const statusDotColor = requiresReconnection
    ? 'error.main'
    : requiresSetup || connection.state === ConnectionState.DISCONNECTING
      ? 'warning.main'
      : isHealthyConfigured
        ? 'success.main'
        : 'text.disabled';

  return {
    createdDate,
    isHealthyConfigured,
    isUnhealthy,
    requiresSetup,
    requiresReconnection,
    statusBadgeLabel,
    statusBadgeColor,
    statusDotColor,
    statusText,
  };
};
