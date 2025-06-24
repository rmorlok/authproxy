import * as React from 'react';
import * as ReactDOM from 'react-dom/client';
import {BrowserRouter} from "react-router-dom";
import {ThemeProvider} from '@emotion/react';
import {CssBaseline} from '@mui/material';
import theme from './theme';
import App from './App';
import { Provider } from 'react-redux';
import { store } from './store';
import {useDispatch} from "react-redux";
import {loadAuthStateAsync, AppDispatch} from "./store";

// Remove?
// import { createTheme, ThemeProvider } from '@mui/material/styles';
//
// const defaultTheme = createTheme();

// Trigger auth state to load as soon as the page loads.
store.dispatch(loadAuthStateAsync());

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
