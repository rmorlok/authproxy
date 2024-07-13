import type {Meta, StoryObj} from '@storybook/react';
import SignIn from "../SignIn";
import {store} from '../store';
import {Provider} from "react-redux";
import {PropsWithChildren} from "react";
import GameBoard from "../GameBoard";
import {WordCard} from "../WordCard";
import {Team, WordCardModel} from "../api/models";

const MockStore = ({children}: PropsWithChildren<any>) => (
    <Provider store={store}>
        {children}
    </Provider>
);

const meta = {
    title: 'Game/WordCard',
    component: WordCard,
    parameters: {
        // More on how to position stories at: https://storybook.js.org/docs/configure/story-layout
        layout: 'centered',
    },
    args: {
        click: (word: string) => alert(`You clicked '${word}'`),
    }
} satisfies Meta<typeof WordCard>;

export default meta;

type Story = StoryObj<typeof meta>;

export const NotRevealed: Story = {
    args: {
        card: {
            word: 'Bear',
            color: 'red',
            revealed: false,
        },
        isSpymaster: false,
        myTeam: 'blue',
    }
};

export const Revealed: Story = {
    args: {
        card: {
            word: 'Bear',
            color: 'blue',
            revealed: true,
        },
        isSpymaster: false,
        myTeam: 'blue',
    }
};

export const LongWord: Story = {
    args: {
        card: {
            word: 'supercalifragilisticexpialidocious',
            color: 'blue',
            revealed: true,
        },
        isSpymaster: false,
        myTeam: 'blue',
    }
};
