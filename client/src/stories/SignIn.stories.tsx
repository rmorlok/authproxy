import type {Meta, StoryObj} from '@storybook/react';
import SignIn from "../SignIn";
import {store} from '../store';
import {Provider} from "react-redux";
import {PropsWithChildren} from "react";

const MockStore = ({children}: PropsWithChildren<any>) => (
    <Provider store={store}>
        {children}
    </Provider>
);

const meta = {
    title: 'SignIn/Screen',
    component: SignIn,
    decorators: [
        (story) => <MockStore>{story()}</MockStore>,
    ],
    parameters: {
        // More on how to position stories at: https://storybook.js.org/docs/configure/story-layout
        layout: 'fullscreen',
    },
} satisfies Meta<typeof SignIn>;

export default meta;

type Story = StoryObj<typeof meta>;

export const LoggedOut: Story = {};
