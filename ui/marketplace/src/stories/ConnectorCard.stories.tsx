import * as React from 'react';
import { Meta, StoryObj } from '@storybook/react';
import ConnectorCard, { ConnectorCardSkeleton } from '../components/ConnectorCard';
import { Connector, ConnectorVersionState } from '@authproxy/api';

const logoDataUri = (label: string, background: string) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${label} logo"><rect width="280" height="140" rx="8" fill="${background}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
};

const meta: Meta<typeof ConnectorCard> = {
  title: 'Components/ConnectorCard',
  component: ConnectorCard,
  parameters: {
    layout: 'centered',
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof ConnectorCard>;

const mockConnector: Connector = {
  id: 'google-calendar',
  version: 1,
  state: ConnectorVersionState.ACTIVE,
  type: 'oauth',
  display_name: 'Google Calendar',
  description: 'Connect to your Google Calendar to manage events and appointments.',
  highlight: 'Manage events and appointments from Google Calendar.',
  logo: logoDataUri('Google Calendar', '#1a73e8'),
  versions: 1,
  states: [ConnectorVersionState.ACTIVE],
};

export const Default: Story = {
  args: {
    connector: mockConnector,
    onConnect: (id) => console.log(`Connect clicked for ${id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const WithHighlight: Story = {
  args: {
    connector: {
      ...mockConnector,
      highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\n\n• Event creation and management\n• Meeting scheduling\n• Reminder notifications\n• Calendar sharing',
    },
    onConnect: (id) => console.log(`Connect clicked for ${id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const Connecting: Story = {
  args: {
    connector: mockConnector,
    onConnect: (id) => console.log(`Connect clicked for ${id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: true,
  },
};

export const LongDescription: Story = {
  args: {
    connector: {
      ...mockConnector,
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.',
      highlight: 'Short marketplace highlight stays on the card while the long description belongs on the overview page.',
    },
    onConnect: (id) => console.log(`Connect clicked for ${id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const Skeleton: Story = {
  render: () => <ConnectorCardSkeleton />,
};
