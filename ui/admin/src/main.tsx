import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import {CssBaseline} from '@mui/material';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import App from './App';
import { Provider } from 'react-redux';
import { store } from './store';
import {initiateSessionAsync} from "./store";
import { configureClient, ApiSessionInitiateRequest } from '@authproxy/api';

// Configure the shared SDK client for the Admin UI
configureClient({
    baseURL: import.meta.env.VITE_ADMIN_BASE_URL,
});

const theme = createTheme({
    colorSchemes: {
        light: true,
        dark: true,
    },
});

// Construct auth parameters from either window variable or URL query parameter
const url = new URL(window.location.href);
const searchParams = url.searchParams;
const authTokenFromQuery = searchParams.get('auth_token');

// If we have an auth token in the query string, remove it from the URL bar immediately
if (authTokenFromQuery) {
    const cleaned = new URL(url.toString());
    cleaned.searchParams.delete('auth_token');
    // Preserve SPA state while cleaning the URL
    window.history.replaceState(window.history.state, document.title, cleaned.toString());
}

const params: ApiSessionInitiateRequest = {
    // Always use the cleaned URL (without auth_token) as the return_to_url
    return_to_url: (authTokenFromQuery ? (() => { const u = new URL(window.location.href); u.searchParams.delete('auth_token'); return u.toString(); })() : window.location.href),
};

// Prefer window-provided token (e.g., server-injected), otherwise use token from query string
if ((window as any).AUTHPROXY_AUTH_TOKEN) {
    params.auth_token = (window as any).AUTHPROXY_AUTH_TOKEN;
} else if (authTokenFromQuery) {
    params.auth_token = authTokenFromQuery;
}

// Trigger auth state to load as soon as the page loads.
store.dispatch(initiateSessionAsync(params));

ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
        <Provider store={store}>
            <ThemeProvider theme={theme}>
                <CssBaseline enableColorScheme />
                <App/>
            </ThemeProvider>
        </Provider>
    </React.StrictMode>,
);
