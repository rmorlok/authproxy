import React from 'react';
import {
  Container,
  Box,
  Button,
  Menu,
  MenuItem,
  Avatar,
  Snackbar,
  Alert,
  Typography,
} from '@mui/material';
import { Outlet } from 'react-router-dom';
import { useDispatch, useSelector } from 'react-redux';
import {terminate, selectActorId, selectToasts, closeToast} from '../store';
import { useState } from 'react';

/**
 * Layout component for the application
 */
const Layout: React.FC = () => {
  const dispatch = useDispatch();
  const actor_id = useSelector(selectActorId);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);
  const toasts = useSelector(selectToasts);

  const handleMenu = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleLogout = () => {
    handleClose();
    dispatch(terminate());
  };

  const toastsContent = toasts.length == 0 ? '' : toasts.map((toast, i) => (
        <Snackbar
            key={toast.id}
            open={true}
            autoHideDuration={6000}
            onClose={() => closeToast(i)}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
        >
          <Alert onClose={() => closeToast(i)} severity={toast.type} sx={{ width: '100%' }}>
            {toast.message}
          </Alert>
        </Snackbar>
      )
  );

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', minHeight: '100vh', bgcolor: 'background.default' }}>
      {actor_id && (
        <Container maxWidth="lg" sx={{ display: 'flex', justifyContent: 'flex-end', pt: { xs: 1, sm: 2 } }}>
          <Button
            id="account-button"
            onClick={handleMenu}
            color="inherit"
            size="small"
            endIcon={(
              <Avatar
                alt={actor_id}
                src="/assets/avatar.png"
                sx={{ width: 28, height: 28, fontSize: 14 }}
              />
            )}
            aria-controls={open ? 'account-menu' : undefined}
            aria-haspopup="true"
            aria-expanded={open ? 'true' : undefined}
            sx={{ color: 'text.secondary', minWidth: 0, textTransform: 'none' }}
          >
            <Typography
              variant="body2"
              component="span"
              noWrap
              sx={{ display: { xs: 'none', sm: 'inline' }, maxWidth: 260 }}
            >
              {actor_id}
            </Typography>
          </Button>
          <Menu
            id="account-menu"
            anchorEl={anchorEl}
            open={open}
            onClose={handleClose}
            MenuListProps={{
              'aria-labelledby': 'account-button',
            }}
          >
            <MenuItem disabled>
              <Typography variant="body2">
                {actor_id}
              </Typography>
            </MenuItem>
            <MenuItem onClick={handleLogout}>Logout</MenuItem>
          </Menu>
        </Container>
      )}
      <Box component="main" sx={{ flexGrow: 1 }}>
        <Outlet />
        {toastsContent}
      </Box>
    </Box>
  );
};

export default Layout;
