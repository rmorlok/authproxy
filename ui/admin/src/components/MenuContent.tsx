import * as React from 'react';
import List from '@mui/material/List';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemIcon from '@mui/material/ListItemIcon';
import ListItemText from '@mui/material/ListItemText';
import Stack from '@mui/material/Stack';
import { Link, useLocation } from 'react-router-dom';
import {adminNavigationItems, type AdminNavigationItem} from '../search/navigation';

const mainListItems = adminNavigationItems.filter((item) => item.section === 'main');
const secondaryListItems = adminNavigationItems.filter((item) => item.section === 'secondary');

function NavigationListItem({item}: {item: AdminNavigationItem}) {
  const location = useLocation();
  const Icon = item.icon;
  const selected = !item.external && location.pathname.startsWith(item.path);
  const content = (
    <>
      <ListItemIcon><Icon /></ListItemIcon>
      <ListItemText primary={item.label} />
    </>
  );

  return (
    <ListItem disablePadding sx={{display: 'block'}}>
      {item.external ? (
        <ListItemButton component="a" href={item.path} target="_blank" rel="noreferrer">
          {content}
        </ListItemButton>
      ) : (
        <ListItemButton selected={selected} component={Link} to={item.path}>
          {content}
        </ListItemButton>
      )}
    </ListItem>
  );
}

export default function MenuContent() {
    return (
    <Stack sx={{ flexGrow: 1, p: 1, justifyContent: 'space-between' }}>
      <List dense>
        {mainListItems.map((item) => <NavigationListItem key={item.path} item={item} />)}
      </List>
      <List dense>
        {secondaryListItems.map((item) => <NavigationListItem key={item.path} item={item} />)}
      </List>
    </Stack>
  );
}
