import React from 'react';
import { Meta, StoryObj } from '@storybook/react';
import ConnectorCard, { ConnectorCardSkeleton } from '../components/ConnectorCard';
import { Connector } from '../models';

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
  display_name: 'Google Calendar',
  description: 'Connect to your Google Calendar to manage events and appointments.',
  logo: 'https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg',
};

export const Default: Story = {
  args: {
    connector: mockConnector,
    onConnect: (id) => console.log(`Connect clicked for ${id}`),
    isConnecting: false,
  },
};

export const Connecting: Story = {
  args: {
    connector: mockConnector,
    onConnect: (id) => console.log(`Connect clicked for ${id}`),
    isConnecting: true,
  },
};

export const LongDescription: Story = {
  args: {
    connector: {
      ...mockConnector,
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.',
    },
    onConnect: (id) => console.log(`Connect clicked for ${id}`),
    isConnecting: false,
  },
};

export const Skeleton: Story = {
  render: () => <ConnectorCardSkeleton />,
};