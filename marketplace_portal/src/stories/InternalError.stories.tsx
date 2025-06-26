import type {Meta, StoryObj} from '@storybook/react';
import {InternalError} from "../InternalError";

const meta = {
    title: 'Common/InternalError',
    component: InternalError,
    parameters: {
        // More on how to position stories at: https://storybook.js.org/docs/configure/story-layout
        layout: 'fullscreen',
    },
} satisfies Meta<typeof InternalError>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
