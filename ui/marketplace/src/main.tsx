import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import App from './App';
import { Provider } from 'react-redux';
import { store } from './store';
import {initiateSessionAsync} from "./store";
import { configureClient, ApiSessionInitiateRequest } from '@authproxy/api';
import MarketplaceThemeProvider from './MarketplaceThemeProvider';
import { loadMarketplaceColorMode } from './marketplaceConfig';

// Configure the shared SDK client for the Public UI (marketplace)
configureClient({
    baseURL: import.meta.env.VITE_PUBLIC_BASE_URL,
});

// Construct auth parameters from either window variable or URL query parameter
const params: ApiSessionInitiateRequest = {
    returnToUrl: window.location.href,
};
if ((window as any).AUTHPROXY_AUTH_TOKEN) {
    params.authToken = (window as any).AUTHPROXY_AUTH_TOKEN;
} else {
    const urlParams = new URLSearchParams(window.location.search);
    const authToken = urlParams.get('authToken');
    if (authToken) {
        params.authToken = authToken;
    }
}

// Trigger auth state to load as soon as the page loads.
// Skip auth in dev mode for the /dev inspection page so cookies/state can be inspected
// without being redirected away.
if (!(import.meta.env.DEV && window.location.pathname === '/dev')) {
    store.dispatch(initiateSessionAsync(params));
}

void loadMarketplaceColorMode().then((colorMode) => {
    ReactDOM.createRoot(document.getElementById('root')!).render(
        <React.StrictMode>
            <Provider store={store}>
                <MarketplaceThemeProvider colorMode={colorMode}>
                    <App/>
                </MarketplaceThemeProvider>
            </Provider>
        </React.StrictMode>,
    );
});
