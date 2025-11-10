import * as js from "@eslint/js";
import * as globals from "globals";
import tseslint from "typescript-eslint";
import * as pluginReact from "eslint-plugin-react";
import markdown from "@eslint/markdown";
import css from "@eslint/css";
import {defineConfig} from "eslint/config";

export default defineConfig([
    {
        files: ["**/*.{js,mjs,cjs,ts,mts,cts,jsx,tsx}"],
        plugins: {js},
        extends: ["js/recommended"],
        languageOptions: {globals: globals.browser}
    },
    tseslint.configs.recommended,
    pluginReact.configs.flat.recommended,
    {
        files: ["**/*.{ts,tsx,mts,cts}"],
        rules: {
            "@typescript-eslint/no-unused-vars": [
                "error",
                {
                    "argsIgnorePattern": "^_",
                    "varsIgnorePattern": "^_",
                    "caughtErrorsIgnorePattern": "^_",
                }
            ],
            "@typescript-eslint/no-explicit-any": [
                "off"
            ]
        }
    },
    {
        files: ["**/*.md"],
        plugins: {markdown},
        language: "markdown/gfm",
        extends: ["markdown/recommended"]
    },
    {
        files: ["**/*.css"],
        plugins: {css},
        language: "css/css",
        extends: ["css/recommended"]
    },
]);
