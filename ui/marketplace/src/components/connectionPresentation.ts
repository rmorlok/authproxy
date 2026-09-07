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
  const createdDate = new Date(connection.metadata.createdAt).toLocaleDateString();
  const hasPendingSetup = Boolean(connection.status.setup?.stepId);
  const lifecycleState = connection.status.lifecycle.state;
  const isUnhealthy =
    lifecycleState === ConnectionState.CONFIGURED &&
    connection.status.health.state === ConnectionHealthState.UNHEALTHY;
  const isHealthyConfigured =
    lifecycleState === ConnectionState.CONFIGURED &&
    !isUnhealthy &&
    !hasPendingSetup;
  const requiresSetup = lifecycleState === ConnectionState.SETUP || hasPendingSetup;
  const requiresReconnection = isUnhealthy || lifecycleState === ConnectionState.DISABLED;
  const statusBadgeLabel = requiresReconnection
    ? 'Requires reconnection'
    : requiresSetup
      ? 'Requires setup'
      : null;
  const statusBadgeColor: 'warning' | 'error' = requiresReconnection ? 'error' : 'warning';
  const statusText = requiresReconnection
    ? 'Reconnection required'
    : requiresSetup
      ? 'Setup required'
      : lifecycleState === ConnectionState.DISCONNECTING
        ? 'Disconnecting'
        : lifecycleState === ConnectionState.DISCONNECTED
          ? 'Disconnected'
          : `Connected on ${createdDate}`;
  const statusDotColor = requiresReconnection
    ? 'error.main'
    : requiresSetup || lifecycleState === ConnectionState.DISCONNECTING
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
