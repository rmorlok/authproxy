# Admin UI

From the repo root, run the following to start the app:

```bash
nvm use
corepack enable
yarn
yarn workspace @authproxy/admin dev
open http://127.0.0.1:5174/
```

View components

```bash
yarn workspace @authproxy/admin storybook
open http://localhost:6007
```
You can also view a [compiled version of the storybook from the last successful build](https://rmorlok.github.io/authproxy/storybook/admin/).

# Theme / Components

The dashboard theme for this app is from 
[MUI Dashboard](https://mui.com/material-ui/getting-started/templates/dashboard/) template. That project is also MIT 
licensed. See link for details.