import type {Meta, StoryObj} from '@storybook/react';
import LoadingPage from "../LoadingPage";

const meta = {
    title: 'Common/LoadingPage',
    component: LoadingPage,
    parameters: {
        // More on how to position stories at: https://storybook.js.org/docs/configure/story-layout
        layout: 'fullscreen',
    },
} satisfies Meta<typeof LoadingPage>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Normal: Story = {};
