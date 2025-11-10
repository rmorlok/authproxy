import * as React from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {fireEvent, render, screen} from '@testing-library/react';
import '@testing-library/jest-dom';
import ConnectorCard, {ConnectorCardSkeleton} from '../components/ConnectorCard';
import {Connector, ConnectorVersionState} from '@authproxy/api';

describe('ConnectorCard', () => {
    const mockConnector: Connector = {
        id: 'google-calendar',
        version: 1,
        state: ConnectorVersionState.ACTIVE,
        type: 'oauth2',
        display_name: 'Google Calendar',
        description: 'Connect to your Google Calendar to manage events and appointments.',
        highlight: undefined,
        logo: 'https://example.com/google-calendar-logo.png',
        versions: 1,
        states: [ConnectorVersionState.ACTIVE],
        created_at: '2023-04-01T12:00:00Z',
        updated_at: '2023-04-01T12:00:00Z',
    };

    const mockOnConnect = vi.fn();

    beforeEach(() => {
        mockOnConnect.mockClear();
    });

    test('renders connector information correctly', () => {
        render(
            <ConnectorCard
                connector={mockConnector}
                onConnect={mockOnConnect}
                isConnecting={false}
            />
        );

        // Check if the connector name is displayed
        expect(screen.getByText('Google Calendar')).toBeInTheDocument();

        // Check if the description is displayed
        expect(screen.getByText('Connect to your Google Calendar to manage events and appointments.')).toBeInTheDocument();

        // Check if the logo is displayed with the correct alt text
        const logoImg = screen.getByAltText('Google Calendar logo');
        expect(logoImg).toBeInTheDocument();
        expect(logoImg).toHaveAttribute('src', 'https://example.com/google-calendar-logo.png');

        // Check if the connect button is displayed
        expect(screen.getByText('Connect')).toBeInTheDocument();
    });

    test('calls onConnect when the connect button is clicked', () => {
        render(
            <ConnectorCard
                connector={mockConnector}
                onConnect={mockOnConnect}
                isConnecting={false}
            />
        );

        // Click the connect button
        fireEvent.click(screen.getByText('Connect'));

        // Check if onConnect was called with the correct connector ID
        expect(mockOnConnect).toHaveBeenCalledTimes(1);
        expect(mockOnConnect).toHaveBeenCalledWith('google-calendar');
    });

    test('disables the connect button when isConnecting is true', () => {
        render(
            <ConnectorCard
                connector={mockConnector}
                onConnect={mockOnConnect}
                isConnecting={true}
            />
        );

        // Check if the connect button is disabled
        const connectButton = screen.getByText('Connect');
        expect(connectButton).toBeDisabled();

        // Click the connect button
        fireEvent.click(connectButton);

        // Check that onConnect was not called
        expect(mockOnConnect).not.toHaveBeenCalled();
    });

    test('renders skeleton correctly', () => {
        render(<ConnectorCardSkeleton/>);

        // Check if the skeleton elements are in the document
        // We can't check for specific text, but we can check for the presence of skeleton elements
        const skeletons = document.querySelectorAll('.MuiSkeleton-root');
        expect(skeletons.length).toBeGreaterThan(0);
    });
});