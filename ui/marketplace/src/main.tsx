import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import {ThemeProvider} from '@emotion/react';
import {CssBaseline} from '@mui/material';
import theme from './theme';
import App from './App';
import { Provider } from 'react-redux';
import { store } from './store';
import { ApiSessionInitiateRequest } from "./api";
import {initiateSessionAsync, AppDispatch} from "./store";


// Construct auth parameters from either window variable or URL query parameter
const params: ApiSessionInitiateRequest = {
    return_to_url: window.location.href,
};
if ((window as any).AUTHPROXY_AUTH_TOKEN) {
    params.auth_token = (window as any).AUTHPROXY_AUTH_TOKEN;
} else {
    const urlParams = new URLSearchParams(window.location.search);
    const authToken = urlParams.get('auth_token');
    if (authToken) {
        params.auth_token = authToken;
    }
}

// Trigger auth state to load as soon as the page loads.
store.dispatch(initiateSessionAsync(params));

ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
        <Provider store={store}>
            <ThemeProvider theme={theme}>
                <CssBaseline/>
                <App/>
            </ThemeProvider>
        </Provider>
    </React.StrictMode>,
);
