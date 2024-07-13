import type { Meta, StoryObj } from '@storybook/react';
import React from 'react'
import {TeamPicker} from "../TeamPicker";
import {Team} from "../api/models";
import {generateFakePlayers} from "./fake_users";


// More on how to set up stories at: https://storybook.js.org/docs/writing-stories#default-export
const meta = {
  title: 'Pregame/TeamPicker',
  component: TeamPicker,
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
} satisfies Meta<typeof TeamPicker>;

export default meta;
type Story = StoryObj<typeof meta>;




// More on writing stories with args: https://storybook.js.org/docs/writing-stories/args
// export const Normal: Story = {
//   args: {
//     tableId: "8675309",
//     player: this_player,
//     players: fake_players,
//     playTeam: (t: Team) => {
//       if(this_player.team === t) {
//         return;
//       }
//       this_player.team = t;
//       this_player.isSpyMaster = false;
//
//       console.log(fake_players)
//     },
//     makeSpymaster: () => {
//       if(this_player.isSpyMaster) {
//         return;
//       }
//
//       fake_players.forEach((p) => {
//         if(p.team === this_player.team) {
//           p.isSpyMaster = false;
//         }
//
//         this_player.isSpyMaster = true;
//       })
//     },
//     leaveTable: () => { alert('You left the table'); },
//     logout: () => { alert('You logged out'); },
//     startGame: () => { alert('You started the game'); },
//   },
// };

export const Normal = () => {
  const {fake_players, this_player} = generateFakePlayers();
  const [players, setPlayers] = React.useState(fake_players);
  const [me, setMe] = React.useState(this_player);

  const props = {
    tableId: "8675309",
    player: me,
    players: players,
    playTeam: (t: Team) => {
      if(me.team === t) {
        return;
      }

      const newMe = {
        ...me,
        team: t,
        isSpyMaster: false,
      };

      setMe(newMe);
      setPlayers(players.map((p) => {
        if (p.user.id === newMe.user.id) {
          return newMe;
        } else {
          return {...p};
        }
      }));
    },
    makeSpymaster: () => {
      if(me.isSpyMaster) {
        return;
      }

      const updatedPlayers = players.map((p) => {
        if(p.user.id !== me.user.id && p.team === this_player.team) {
          return {
            ...p,
            isSpyMaster: false,
          };
        } else if (p.user.id === me.user.id) {
          return {
            ...p,
            isSpyMaster: true,
          };
        } else {
          return {...p};
        }
      });
      setPlayers(updatedPlayers);
      setMe({...me, isSpyMaster: true});
    },
    leaveTable: () => { alert('You left the table'); },
    logout: () => { alert('You logged out'); },
    startGame: () => { alert('You started the game'); },
  };

  return (<TeamPicker {...props} />);
};