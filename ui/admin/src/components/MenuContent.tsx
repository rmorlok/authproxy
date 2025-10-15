import * as React from 'react';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Stack from '@mui/material/Stack';
import { Link, useLocation } from 'react-router-dom';
import HomeRoundedIcon from '@mui/icons-material/HomeRounded';
import PowerRoundedIcon from '@mui/icons-material/Power';
import LinkRoundedIcon from '@mui/icons-material/Link';
import HttpRoundedIcon from '@mui/icons-material/Http';
import PeopleRoundedIcon from '@mui/icons-material/PeopleRounded';
import AssignmentRoundedIcon from '@mui/icons-material/AssignmentRounded';
import SettingsRoundedIcon from '@mui/icons-material/SettingsRounded';
import InfoRoundedIcon from '@mui/icons-material/InfoRounded';
import HelpRoundedIcon from '@mui/icons-material/HelpRounded';

const mainListItems = [
  { text: 'Home', icon: <HomeRoundedIcon />, link: '/home' },
  { text: 'Connectors', icon: <PowerRoundedIcon />, link: '/connectors' },
  { text: 'Connections', icon: <LinkRoundedIcon />, link: '/connections' },
  { text: 'Requests', icon: <HttpRoundedIcon />, link: '/requests' },
  { text: 'Tasks', icon: <AssignmentRoundedIcon />, link: '/tasks' },
  { text: 'Actors', icon: <PeopleRoundedIcon />, link: '/actors' },
];

const secondaryListItems = [
  { text: 'Settings', icon: <SettingsRoundedIcon />, link: '/settings'},
  { text: 'About', icon: <InfoRoundedIcon />, link: '/about' },
  { text: 'Feedback', icon: <HelpRoundedIcon />, link: 'https://github.com/rmorlok/authproxy/issues' },
];

export default function MenuContent() {
    const location = useLocation();

    return (
    <Stack sx={{ flexGrow: 1, p: 1, justifyContent: 'space-between' }}>
      <List dense>
        {mainListItems.map((item, index) => (
          <ListItem key={index} disablePadding sx={{ display: 'block' }}>
            <ListItemButton selected={location.pathname.startsWith(item.link)} component={Link} to={item.link}>
              <ListItemIcon>{item.icon}</ListItemIcon>
              <ListItemText primary={item.text} />
            </ListItemButton>
          </ListItem>
        ))}
      </List>
      <List dense>
        {secondaryListItems.map((item, index) => (
          <ListItem key={index} disablePadding sx={{ display: 'block' }}>
            <ListItemButton selected={location.pathname.startsWith(item.link)}  component={Link} to={item.link}>
              <ListItemIcon>{item.icon}</ListItemIcon>
              <ListItemText primary={item.text} />
            </ListItemButton>
          </ListItem>
        ))}
      </List>
    </Stack>
  );
}
