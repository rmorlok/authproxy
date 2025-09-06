import type {Meta, StoryObj} from '@storybook/react';
import {Error} from "../Error";

const meta = {
  title: 'Common/Error',
  component: Error,
  parameters: {
    // More on how to position stories at: https://storybook.js.org/docs/configure/story-layout
    layout: 'fullscreen',
  },
} satisfies Meta<typeof Error>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
