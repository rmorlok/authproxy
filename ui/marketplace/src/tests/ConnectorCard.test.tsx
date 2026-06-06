import * as React from 'react';
import {beforeEach, describe, expect, test, vi} from 'vitest';
import {fireEvent, render, screen} from '@testing-library/react';
import '@testing-library/jest-dom';
import ConnectorCard, {ConnectorCardSkeleton} from '../components/ConnectorCard';
import {Connector, ConnectorVersionState} from '@authproxy/api';

describe('ConnectorCard', () => {
    const mockConnector: Connector = {
        id: 'google-calendar',
        namespace: 'root',
        version: 1,
        state: ConnectorVersionState.ACTIVE,
        display_name: 'Google Calendar',
        description: 'Connect to your Google Calendar to manage events and appointments.',
        highlight: 'Manage events and appointments from Google Calendar.',
        logo: 'https://example.com/google-calendar-logo.png',
        has_configure: false,
        versions: 1,
        states: [ConnectorVersionState.ACTIVE],
        created_at: '2023-04-01T12:00:00Z',
        updated_at: '2023-04-01T12:00:00Z',
    };

    const mockOnConnect = vi.fn();
    const mockOnDetails = vi.fn();

    beforeEach(() => {
        mockOnConnect.mockClear();
        mockOnDetails.mockClear();
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

        expect(screen.getByText('Manage events and appointments from Google Calendar.')).toBeInTheDocument();
        expect(screen.queryByText('Connect to your Google Calendar to manage events and appointments.')).not.toBeInTheDocument();

        // Check if the logo is displayed with the correct alt text
        const logoImg = screen.getByAltText('Google Calendar logo');
        expect(logoImg).toBeInTheDocument();
        expect(logoImg).toHaveAttribute('src', 'https://example.com/google-calendar-logo.png');

        // Check if the connect button is displayed
        expect(screen.getByText('Connect')).toBeInTheDocument();
    });

    test('calls onDetails when the card or details button is clicked', () => {
        render(
            <ConnectorCard
                connector={mockConnector}
                onConnect={mockOnConnect}
                onDetails={mockOnDetails}
                isConnecting={false}
            />
        );

        fireEvent.click(screen.getByRole('button', {name: /View Google Calendar details/i}));
        expect(mockOnDetails).toHaveBeenCalledWith('google-calendar');

        fireEvent.click(screen.getByRole('button', {name: /^Details$/i}));
        expect(mockOnDetails).toHaveBeenCalledTimes(2);
    });

    test('does not fall back to the full description when the highlight is absent', () => {
        render(
            <ConnectorCard
                connector={{...mockConnector, highlight: undefined}}
                onConnect={mockOnConnect}
                isConnecting={false}
            />
        );

        expect(screen.getByText('Google Calendar')).toBeInTheDocument();
        expect(screen.queryByText('Connect to your Google Calendar to manage events and appointments.')).not.toBeInTheDocument();
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

    test('renders initials when the connector has no logo', () => {
        render(
            <ConnectorCard
                connector={{...mockConnector, display_name: 'No Logo Connector', logo: ''}}
                onConnect={mockOnConnect}
                isConnecting={false}
            />
        );

        expect(screen.getByLabelText('No Logo Connector logo')).toHaveTextContent('NL');
    });

    test('falls back to initials when the connector logo fails to load', () => {
        render(
            <ConnectorCard
                connector={mockConnector}
                onConnect={mockOnConnect}
                isConnecting={false}
            />
        );

        const logoImg = screen.getByAltText('Google Calendar logo');
        fireEvent.error(logoImg);

        expect(screen.getByLabelText('Google Calendar logo')).toHaveTextContent('GC');
    });

    test('renders skeleton correctly', () => {
        render(<ConnectorCardSkeleton/>);

        // Check if the skeleton elements are in the document
        // We can't check for specific text, but we can check for the presence of skeleton elements
        const skeletons = document.querySelectorAll('.MuiSkeleton-root');
        expect(skeletons.length).toBeGreaterThan(0);
    });
});
