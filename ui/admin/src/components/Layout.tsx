import React from 'react';
import {
  AppBar,
  Toolbar,
  Typography,
  Container,
  Box,
  Button,
  IconButton,
  Menu,
  MenuItem,
  Avatar, Snackbar, Alert
} from '@mui/material';
import { Link, Outlet } from 'react-router-dom';
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
    <Box sx={{ display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
      <AppBar position="static">
        <Toolbar>
          <Typography variant="h6" component="div" sx={{ flexGrow: 1 }}>
            <Link to="/" style={{ color: 'inherit', textDecoration: 'none' }}>
              AuthProxy
            </Link>
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <Button 
              color="inherit" 
              component={Link} 
              to="/connectors"
              sx={{ mr: 2 }}
            >
              Connectors
            </Button>
            <Button 
              color="inherit" 
              component={Link} 
              to="/connections"
              sx={{ mr: 2 }}
            >
              Connections
            </Button>
            {actor_id && (
              <>
                <IconButton
                  onClick={handleMenu}
                  color="inherit"
                  size="small"
                  sx={{ ml: 2 }}
                  aria-controls={open ? 'account-menu' : undefined}
                  aria-haspopup="true"
                  aria-expanded={open ? 'true' : undefined}
                >
                  <Avatar 
                    alt={actor_id}
                    src="/assets/avatar.png"
                    sx={{ width: 32, height: 32 }}
                  />
                </IconButton>
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
              </>
            )}
          </Box>
        </Toolbar>
      </AppBar>
      <Box component="main" sx={{ flexGrow: 1 }}>
        <Outlet />
        {toastsContent}
      </Box>
      <Box component="footer" sx={{ py: 3, bgcolor: 'background.paper', mt: 'auto' }}>
        <Container maxWidth="lg">
          <Typography variant="body2" color="text.secondary" align="center">
            AuthProxy &copy; {new Date().getFullYear()}
          </Typography>
        </Container>
      </Box>
    </Box>
  );
};

export default Layout;