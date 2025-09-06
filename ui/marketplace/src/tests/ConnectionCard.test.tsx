import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import ConnectionCard, { ConnectionCardSkeleton } from '../components/ConnectionCard';
import { Connection, ConnectionState } from '../models';
import connectorsReducer from '../store/connectorsSlice';

// Create a mock store with connectors
const createMockStore = (connectors = []) => {
  return configureStore({
    reducer: {
      connectors: connectorsReducer,
    },
    preloadedState: {
      connectors: {
        items: connectors,
        status: 'succeeded',
        error: null,
      },
    },
  });
};

describe('ConnectionCard', () => {
  const mockConnection: Connection = {
    id: '123e4567-e89b-12d3-a456-426614174000',
    connector_id: 'google-calendar',
    state: ConnectionState.CONNECTED,
    created_at: '2023-04-01T12:00:00Z',
    updated_at: '2023-04-01T12:00:00Z',
  };

  const mockConnector = {
    id: 'google-calendar',
    display_name: 'Google Calendar',
    description: 'Connect to your Google Calendar to manage events and appointments.',
    logo: 'https://example.com/google-calendar-logo.png',
  };

  test('renders connection information correctly with connector details', () => {
    const mockStore = createMockStore([mockConnector]);

    render(
      <Provider store={mockStore}>
        <ConnectionCard connection={mockConnection} />
      </Provider>
    );

    // Check if the connector name is displayed
    expect(screen.getByText('Google Calendar')).toBeInTheDocument();
    
    // Check if the connection date is displayed
    expect(screen.getByText(/Connected on/)).toBeInTheDocument();
    
    // Check if the status is displayed
    expect(screen.getByText('Status:')).toBeInTheDocument();
    expect(screen.getByText('connected')).toBeInTheDocument();
    
    // Check if the connection ID is displayed
    expect(screen.getByText(/ID:/)).toBeInTheDocument();
    expect(screen.getByText(/123e4567-e89b-12d3-a456-426614174000/)).toBeInTheDocument();
  });

  test('renders with unknown connector when connector is not found', () => {
    const mockStore = createMockStore([]);

    render(
      <Provider store={mockStore}>
        <ConnectionCard connection={mockConnection} />
      </Provider>
    );

    // Check if the unknown connector text is displayed
    expect(screen.getByText('Unknown Connector')).toBeInTheDocument();
  });

  test('renders different status colors based on connection state', () => {
    const states = [
      { state: ConnectionState.CONNECTED, label: 'connected' },
      { state: ConnectionState.CREATED, label: 'created' },
      { state: ConnectionState.FAILED, label: 'failed' },
      { state: ConnectionState.DISCONNECTED, label: 'disconnected' },
    ];

    states.forEach(({ state, label }) => {
      const mockStore = createMockStore([mockConnector]);
      const connection = { ...mockConnection, state };

      const { unmount } = render(
        <Provider store={mockStore}>
          <ConnectionCard connection={connection} />
        </Provider>
      );

      // Check if the status label is displayed
      expect(screen.getByText(label)).toBeInTheDocument();

      unmount();
    });
  });

  test('renders skeleton correctly', () => {
    render(<ConnectionCardSkeleton />);
    
    // Check if the skeleton elements are in the document
    const skeletons = document.querySelectorAll('.MuiSkeleton-root');
    expect(skeletons.length).toBeGreaterThan(0);
  });
});