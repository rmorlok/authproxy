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
        'AuthProxy is an open-source, self-hosted service that manages third-party API credentials for your application and adds them to the requests it sends.',
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
          label: 'Start here',
          items: [
            { label: 'Documentation home', link: '/' },
            { label: 'How AuthProxy works', link: '/concepts/' },
            'getting-started/demo',
            'development/quick-start',
            'getting-started/first-connection',
          ],
        },
        {
          label: 'Concepts',
          items: [
            'concepts/core-model',
            'concepts/labels-and-annotations',
          ],
        },
        {
          label: 'Build an integration',
          items: [
            'integration',
            'integration/host-application',
            'integration/authentication',
            'integration/marketplace',
            'integration/connectors',
            'integration/connector-setup-flow',
            'integration/connector-predicates',
          ],
        },
        {
          label: 'Call third-party APIs',
          items: [
            'sdks',
            'sdks/proxying',
            'sdks/javascript',
          ],
        },
        {
          label: 'Deploy',
          items: [
            'deployment',
            'deployment/helm',
            'deployment/container-images',
            'deployment/kustomize',
            'deployment/eks-runbook',
          ],
        },
        {
          label: 'Operate',
          items: [
            'operations',
            'operations/app-metrics',
            'operations/telemetry',
            'operations/rate-limits',
            'operations/connector-lifecycle',
            'operations/connector-generation-migrations',
            'operations/background-tasks',
            'operations/migrations',
            'operations/blob-storage',
            'operations/load-testing',
            'operations/redis-insight',
          ],
        },
        {
          label: 'Security',
          items: [
            'security',
            'security/authentication-and-authorization',
            'security/permission-resources-and-verbs',
            'security/encryption',
          ],
        },
        {
          label: 'Reference',
          items: [
            'reference',
            'reference/resources',
            'reference/api',
            'reference/configuration',
            'development/cli',
            'reference/related-projects',
          ],
        },
        {
          label: 'Contribute',
          collapsed: true,
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
                'development/design/tools-and-toolsets',
                'development/design/kubernetes-resource-api',
                'development/design/key-model-migration',
                'development/design/oauth-provider-identity',
              ],
            },
          ],
        },
      ],
    }),
  ],
});
