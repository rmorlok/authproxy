import type { Meta, StoryObj } from '@storybook/react';
import {TablePicker} from "../TablePicker";


// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Pregame/TablePicker',
  component: TablePicker,
  parameters: {
    // Optional parameter to center the component in the Canvas. More info: https://storybook.js.org/docs/configure/story-layout
    layout: 'centered',
  },
  // This component will have an automatically generated Autodocs entry: https://storybook.js.org/docs/writing-docs/autodocs
  tags: ['autodocs'],
  // More on argTypes: https://storybook.js.org/docs/api/argtypes
  argTypes: {
  },
  args: { },
} satisfies Meta<typeof TablePicker>;

export default meta;
type Story = StoryObj<typeof meta>;

// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
export const Normal: Story = {
  args: {
    logout: () => { alert('You logged out'); },
    createTable: () => { alert('You created a table'); },
    joinTable: (id) => { alert(`You joined table ${id}`); },
    validateTableId: async (id) => {
      return new Promise<boolean>((resolve, _reject) => {
        console.log("Validating Table ID");
        setTimeout(() => {
          resolve(id === '8675309');
        }, 2000);
      });
    },
  },
};
