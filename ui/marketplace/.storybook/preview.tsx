import type { Preview } from "@storybook/react";
import React from "react";
import { MemoryRouter } from "react-router-dom";

const customViewports = {
  marketplaceMobile: {
    name: 'Marketplace mobile',
    styles: {
      width: '390px',
      height: '844px',
    },
  },
  marketplaceTablet: {
    name: 'Marketplace tablet',
    styles: {
      width: '834px',
      height: '1112px',
    },
  },
  marketplaceDesktop: {
    name: 'Marketplace desktop',
    styles: {
      width: '1440px',
      height: '900px',
    },
  },
};

const preview: Preview = {
  decorators: [
    (Story) => (
      <MemoryRouter>
        <Story />
      </MemoryRouter>
    ),
  ],
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    viewport: {
      viewports: customViewports,
    },
  },
};

export default preview;
