import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import astroMermaid from 'astro-mermaid';

export default defineConfig({
  site: 'https://docs.authproxy.net',
  integrations: [
    astroMermaid({
      autoTheme: true,
      enableLog: false,
      mermaidConfig: {
        securityLevel: 'strict',
      },
    }),
    starlight({
      title: 'AuthProxy',
      description:
        'Create, secure, use, and operate third-party API connections from one integration platform.',
      customCss: ['./src/styles/custom.css'],
      lastUpdated: true,
      editLink: {
        baseUrl: 'https://github.com/rmorlok/authproxy/edit/main/docs/',
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/rmorlok/authproxy',
        },
      ],
      sidebar: [
        {
          label: 'Get started',
          items: [
            { label: 'Documentation home', link: '/' },
            'getting-started/demo',
            'development/quick-start',
          ],
        },
        {
          label: 'Concepts',
          items: [
            'concepts',
            'concepts/core-model',
            'concepts/labels-and-annotations',
          ],
        },
        {
          label: 'Integrate',
          items: [
            'integration',
            'integration/host-application',
            'integration/marketplace',
            'integration/connector-setup-flow',
            'integration/connector-predicates',
          ],
        },
        {
          label: 'SDKs & API',
          items: [
            'sdks',
            'sdks/proxying',
            'sdks/javascript',
            'development/cli',
            'reference/api',
          ],
        },
        {
          label: 'Deploy',
          items: [
            'deployment',
            'deployment/helm',
            'deployment/kustomize',
            'deployment/container-images',
            'deployment/eks-runbook',
          ],
        },
        {
          label: 'Operate',
          items: [
            'operations',
            'operations/telemetry',
            'operations/app-metrics',
            'operations/rate-limits',
            'operations/background-tasks',
            'operations/connector-lifecycle',
            'operations/blob-storage',
            'operations/redis-insight',
          ],
        },
        {
          label: 'Security',
          items: [
            'security',
            'security/authentication-and-authorization',
            'security/encryption',
          ],
        },
        {
          label: 'Develop',
          items: [
            'development',
            'development/local-development',
            'development/codebase',
            'development/workflows',
            {
              label: 'Design notes',
              collapsed: true,
              items: [
                'development/design',
                'development/design/key-model-migration',
                'development/design/oauth-provider-identity',
              ],
            },
          ],
        },
        {
          label: 'Reference',
          collapsed: true,
          items: [
            'reference',
            'reference/configuration',
            'reference/related-projects',
          ],
        },
      ],
    }),
  ],
});
