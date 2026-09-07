import * as React from 'react';
import { Meta, StoryObj } from '@storybook/react';
import ConnectorCard, { ConnectorCardSkeleton } from '../components/ConnectorCard';
import { Connector } from '@authproxy/api';
import { connectorFixture } from '../testing/resources';

const logoDataUri = (label: string, background: string) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${label} logo"><rect width="280" height="140" rx="8" fill="${background}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
};

const wideLogoDataUri = (label: string) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${label} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;
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

const mockConnector: Connector = connectorFixture({
  displayName: 'Google Calendar',
  description: 'Connect to your Google Calendar to manage events and appointments.',
  highlight: 'Manage events and appointments from Google Calendar.',
  logo: {publicUrl: logoDataUri('Google Calendar', '#1a73e8')},
});

export const Default: Story = {
  args: {
    connector: mockConnector,
    onConnect: (connector) => console.log(`Connect clicked for ${connector.metadata.id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const WithHighlight: Story = {
  args: {
    connector: {
      ...mockConnector,
      spec: {
        ...mockConnector.spec,
        definition: {
          ...mockConnector.spec.definition,
          highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\n\n• Event creation and management\n• Meeting scheduling\n• Reminder notifications\n• Calendar sharing',
        },
      },
    },
    onConnect: (connector) => console.log(`Connect clicked for ${connector.metadata.id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const Connecting: Story = {
  args: {
    connector: mockConnector,
    onConnect: (connector) => console.log(`Connect clicked for ${connector.metadata.id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: true,
  },
};

export const LongDescription: Story = {
  args: {
    connector: connectorFixture({
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.',
      highlight: 'Short marketplace highlight stays on the card while the long description belongs on the overview page.',
      logo: {publicUrl: logoDataUri('Google Calendar', '#1a73e8')},
    }),
    onConnect: (connector) => console.log(`Connect clicked for ${connector.metadata.id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const DescriptionFallback: Story = {
  args: {
    connector: connectorFixture({
      description: 'Allow the agent to manage your calendar on your behalf. It is like having your own personal assistant.',
      logo: {publicUrl: logoDataUri('Google Calendar', '#1a73e8')},
    }),
    onConnect: (connector) => console.log(`Connect clicked for ${connector.metadata.id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const WideLogo: Story = {
  args: {
    connector: connectorFixture({
      id: 'wide-format-systems',
      displayName: 'Wide Format Systems',
      highlight: 'A wide logo should scale down inside the card without being cut off.',
      logo: {publicUrl: wideLogoDataUri('Wide Format Systems')},
    }),
    onConnect: (connector) => console.log(`Connect clicked for ${connector.metadata.id}`),
    onDetails: (id) => console.log(`Details clicked for ${id}`),
    isConnecting: false,
  },
};

export const Skeleton: Story = {
  render: () => <ConnectorCardSkeleton />,
};
