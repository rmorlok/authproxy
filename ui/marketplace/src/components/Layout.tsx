import React, {useEffect, useMemo, useState} from 'react';
import {
  Avatar,
  Badge,
  Box,
  Button,
  Container,
  Divider,
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Alert,
  Snackbar,
  Tooltip,
  Typography,
} from '@mui/material';
import { Notification, NotificationLevel } from '@authproxy/api';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import NotificationsNoneIcon from '@mui/icons-material/NotificationsNone';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import { Outlet, useNavigate } from 'react-router-dom';
import { useDispatch, useSelector } from 'react-redux';
import {
  AppDispatch,
  closeToast,
  fetchNotificationsAsync,
  markNotificationsViewedAsync,
  selectActorId,
  selectNotifications,
  selectNotificationsError,
  selectNotificationsStatus,
  selectToasts,
  selectUnviewedNotificationCount,
  terminate,
} from '../store';
import { marketplaceTokens } from '../theme';

const notificationIcon = (notification: Notification) => {
  if (notification.level === NotificationLevel.ERROR) {
    return <ErrorOutlineIcon color="error" fontSize="small" />;
  }
  if (notification.level === NotificationLevel.WARNING) {
    return <WarningAmberIcon color="warning" fontSize="small" />;
  }
  return <InfoOutlinedIcon color="info" fontSize="small" />;
};

const routeFromActionUrl = (actionUrl: string): string | null => {
  try {
    const url = new URL(actionUrl, window.location.origin);
    if (url.origin !== window.location.origin) {
      return null;
    }
    return `${url.pathname}${url.search}${url.hash}`;
  } catch (_error) {
    return null;
  }
};

/**
 * Layout component for the application
 */
const Layout: React.FC = () => {
  const dispatch = useDispatch<AppDispatch>();
  const navigate = useNavigate();
  const actor_id = useSelector(selectActorId);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [notificationsAnchorEl, setNotificationsAnchorEl] = useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);
  const notificationsOpen = Boolean(notificationsAnchorEl);
  const toasts = useSelector(selectToasts);
  const notifications = useSelector(selectNotifications);
  const notificationsStatus = useSelector(selectNotificationsStatus);
  const notificationsError = useSelector(selectNotificationsError);
  const unviewedNotificationCount = useSelector(selectUnviewedNotificationCount);
  const unviewedNotificationIds = useMemo(
    () => notifications.filter((item) => !item.viewed).map((item) => item.id),
    [notifications]
  );

  useEffect(() => {
    if (actor_id && notificationsStatus === 'idle') {
      dispatch(fetchNotificationsAsync());
    }
  }, [actor_id, dispatch, notificationsStatus]);

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

  const handleNotificationsOpen = (event: React.MouseEvent<HTMLElement>) => {
    setNotificationsAnchorEl(event.currentTarget);
    if (unviewedNotificationIds.length > 0) {
      dispatch(markNotificationsViewedAsync(unviewedNotificationIds));
    }
  };

  const handleNotificationsClose = () => {
    setNotificationsAnchorEl(null);
  };

  const handleNotificationAction = (notification: Notification) => {
    if (!notification.action_url || !notification.can_action) {
      return;
    }
    handleNotificationsClose();

    const route = routeFromActionUrl(notification.action_url);
    if (route) {
      navigate(route);
      return;
    }
    window.location.href = notification.action_url;
  };

  const toastsContent = toasts.length == 0 ? '' : toasts.map((toast, i) => (
        <Snackbar
            key={toast.id}
            open={true}
            autoHideDuration={6000}
            onClose={() => dispatch(closeToast(i))}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
        >
          <Alert onClose={() => dispatch(closeToast(i))} severity={toast.type} sx={{ width: '100%' }}>
            {toast.message}
          </Alert>
        </Snackbar>
      )
  );

  const notificationsContent = notifications.length === 0 ? (
    <MenuItem disabled>
      <ListItemText
        primary="No notifications"
        primaryTypographyProps={{ variant: 'body2', color: 'text.secondary' }}
      />
    </MenuItem>
  ) : notifications.map((notification) => (
    <MenuItem
      key={notification.id}
      disableRipple
      sx={{
        alignItems: 'flex-start',
        gap: 1.5,
        maxWidth: 420,
        minWidth: { xs: 300, sm: 380 },
        py: 1.5,
        whiteSpace: 'normal',
      }}
    >
      <ListItemIcon sx={{ minWidth: 32, pt: 0.25 }}>
        {notificationIcon(notification)}
      </ListItemIcon>
      <ListItemText
        primary={notification.title}
        secondary={notification.message}
        primaryTypographyProps={{
          variant: 'subtitle2',
          fontWeight: notification.viewed ? 500 : 700,
        }}
        secondaryTypographyProps={{
          variant: 'body2',
          color: 'text.secondary',
          sx: { mt: 0.5 },
        }}
      />
      {notification.can_action && notification.action_url && (
        <Button
          size="small"
          variant="outlined"
          onClick={(event) => {
            event.stopPropagation();
            handleNotificationAction(notification);
          }}
          sx={{ flexShrink: 0, mt: 0.25 }}
        >
          Open
        </Button>
      )}
    </MenuItem>
  ));

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', minHeight: '100vh', bgcolor: 'background.default' }}>
      {actor_id && (
        <Container
          maxWidth="lg"
          sx={{
            display: 'flex',
            justifyContent: 'flex-end',
            alignItems: 'center',
            gap: 1,
            pt: { xs: 1, sm: 2 },
          }}
        >
          <Tooltip title="Open notifications">
            <IconButton
              id="notifications-button"
              color="inherit"
              size="small"
              onClick={handleNotificationsOpen}
              aria-controls={notificationsOpen ? 'notifications-menu' : undefined}
              aria-haspopup="true"
              aria-expanded={notificationsOpen ? 'true' : undefined}
              aria-label="Open notifications"
              sx={{ color: 'text.secondary' }}
            >
              <Badge
                badgeContent={unviewedNotificationCount}
                color="warning"
                invisible={unviewedNotificationCount === 0}
              >
                <NotificationsNoneIcon fontSize="small" />
              </Badge>
            </IconButton>
          </Tooltip>
          <Menu
            id="notifications-menu"
            anchorEl={notificationsAnchorEl}
            open={notificationsOpen}
            onClose={handleNotificationsClose}
            MenuListProps={{ 'aria-labelledby': 'notifications-button' }}
            PaperProps={{
              sx: {
                mt: 1,
                maxHeight: 480,
                borderRadius: marketplaceTokens.radius.panel,
              },
            }}
          >
            <Box sx={{ px: 2, py: 1.25 }}>
              <Typography variant="subtitle1" component="p" sx={{ fontWeight: 700 }}>
                Notifications
              </Typography>
              {notificationsStatus === 'failed' && (
                <Typography variant="body2" color="error">
                  {notificationsError ?? 'Could not load notifications'}
                </Typography>
              )}
            </Box>
            <Divider />
            {notificationsContent}
          </Menu>
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
