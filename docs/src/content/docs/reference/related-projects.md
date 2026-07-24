---
title: Related projects
description: Compare AuthProxy with unified APIs, embedded iPaaS products, API and MCP gateways, workflow platforms, and open-source integration frameworks.
---

This is a research index for products adjacent to AuthProxy. Vendor packaging,
self-hosting terms, and connector counts change frequently; verify the linked
vendor sources before using the tables for a procurement decision.

The landscape splits into a few overlapping domains:

- **Unified APIs**: Normalize a vertical (e.g., HRIS/ATS/Accounting) behind a single data model and endpoint set.
- **Embedded iPaaS**: Integration infrastructure designed to be embedded into your SaaS product for customer-facing integrations.
- **Traditional iPaaS / Workflow Automation**: Internal automation platforms with large connector catalogs and visual builders.
- **Webhook / Event Gateways**: Reliability infrastructure for receiving, routing, observing, replaying, and delivering asynchronous events.
- **API, AI, and MCP Gateways**: Traffic policy, model routing, tool aggregation, or identity-aware access for APIs and agent infrastructure.
- **Open-source Frameworks / Building Blocks**: Libraries or frameworks for teams that want full control.

Below are summaries and comparison tables for the projects in this repo's list. Connector counts are taken from the vendors' public catalogs or docs when available; when not available, it's noted explicitly.

## Where AuthProxy fits

AuthProxy is a strong fit when a team wants to keep using native third-party
APIs, run the credential data plane in infrastructure it controls, and reuse a
connection lifecycle across many integrations. It provides embedded connection
and administration UIs, but the host application keeps its own business logic
and provider-specific requests.

That produces different tradeoffs from adjacent categories:

- A **unified API** reduces provider differences but introduces a normalized
  data model. AuthProxy preserves the provider's full native surface.
- A **managed embedded iPaaS** can provide a larger maintained catalog and
  hosted operations. AuthProxy provides source access and deployment control,
  while the adopting team owns its connectors and environment.
- A **workflow automation system** owns orchestration and transformations.
  AuthProxy focuses on connections, authentication, proxying, and governance;
  application code still owns the workflow.
- A **webhook or event gateway** makes asynchronous event ingress or egress
  reliable with queues, retries, replay, routing, and delivery logs. AuthProxy
  manages credentials and authenticated synchronous API calls; it does not
  replace durable webhook delivery infrastructure.
- An **API or MCP traffic gateway** applies authentication, authorization,
  rate limits, routing, and observability at a shared network boundary.
  AuthProxy instead owns each application's tenant-facing connection setup,
  provider credential lifecycle, and authenticated native-API forwarding.
- A **secret manager or credential proxy** protects secret access. AuthProxy
  adds OAuth callbacks and refresh, user-facing setup, connector versions,
  health, rate limits, and request-event context.

The relevant build-versus-buy question is therefore not only connector count.
It is whether the team wants to own provider-specific integration behavior and
its deployment in exchange for native API access, extensibility, and control of
credential and request data.

---

## Unified APIs

| Product | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [Merge](https://www.merge.dev/) | Commercial | Primarily embedded (customer-facing) | 220+ integrations | Yes (paid option; details not public) | Unified APIs across HRIS, ATS, CRM, Accounting, Ticketing, etc. | 
| [Kombo](https://www.kombo.dev/) | Commercial | Embedded | 250+ integrations | No public self-host option; regional SaaS (EU/US) | Unified API focused on HRIS/ATS/LMS/Payroll. |

### Unified API product notes

- **Merge**: Provides unified APIs across multiple verticals plus a hosted platform that manages auth, maintenance, and monitoring. Connector count published in their integrations catalog. Supports webhooks and standard API patterns in docs. Code-first via REST APIs, with SDKs and platform tooling. Pricing indicates a self-hosted option available for purchase. See: https://www.merge.dev/integrations, https://docs.merge.dev/, and https://www.merge.dev/pricing.
- **Kombo**: Unified API focused on HR/ATS/LMS/Payroll. Connector counts published in their integrations catalog and category pages. Built for embedding into SaaS products. Docs describe EU/US regional hosting, with no self-host option mentioned. See: https://www.kombo.dev/integrations and https://docs.kombo.dev/data-residency-compliance.

---

## Embedded iPaaS (customer-facing integrations)

| Product | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [Paragon](https://www.useparagon.com/) | Commercial | Embedded | 130+ native connectors | Yes (managed or unmanaged on-prem) | Embedded SDK, white-labeled UI, auth, logs, multi-tenant. |
| [Prismatic](https://prismatic.io/) | Commercial | Embedded | 190+ connectors | Partial (on-prem agent for private resources) | Low-code + code SDK, integration marketplace tooling. |
| [Workato](https://www.workato.com/) | Commercial | Traditional iPaaS w/ embedded option | 1,000+ connectors | Partial (on-prem agent for private systems) | Enterprise automation platform; embedded option exists. |
| [Tray.io](https://tray.io/) | Commercial | Traditional + embedded offering | Not publicly stated | Partial (on-prem agent for private systems) | Visual workflow builder + embedded product. |
| [Cyclr](https://cyclr.com/) | Commercial | Embedded | 600+ applications | Yes (self-hosted private cloud) | Embedded integration platform and connector library. |
| [Pandium](https://www.pandium.com/) | Commercial | Embedded | 200+ connectors (docs mention 210) | No public self-host option; managed infrastructure | Code-first, connectors focus on auth + webhooks. |
| [Ampersand](https://www.withampersand.com/) | Commercial + OSS connectors | Embedded | Not publicly stated | No public self-host for platform; OSS connectors library | Declarative, code-first integrations; open connectors repo. |
| [Nango](https://nango.dev/) | Commercial + OSS (Elastic License) | Embedded / integration infrastructure | 500-700+ APIs (varies by page) | Yes (limited free self-host; enterprise self-host) | Code-first integration platform with auth, sync, webhooks. |
| [Frigg](https://friggframework.org/) | Open Source | Embedded framework | Not publicly stated | Yes (runs in your cloud) | OSS framework for building integrations in your own cloud. |

### Embedded iPaaS product notes

- **Paragon**: Embedded integration platform with 130+ native connectors plus a custom integration builder, white-labeled connect portal, multi-tenant auth, and logs. Paragon supports cloud, managed on-prem, and unmanaged on-prem deployments. See: https://docs.useparagon.com/overview and https://docs.useparagon.com/on-premise/choosing-a-hosting-solution.
- **Prismatic**: Embedded iPaaS with built-in connectors and SDK for custom ones; integration marketplace tooling. Connectors page lists 190+ built-in connectors. An on-prem agent (Docker container) can run in a private network to access private resources. See: https://prismatic.io/connectors/ and https://prismatic.io/docs/integrations/connections/on-prem-agent/.
- **Workato**: Traditional enterprise iPaaS with an embedded option; docs list 1,000+ connectors. On-premise agents provide secure access to private systems. See: https://docs.workato.com/connectors and https://docs.workato.com/on-prem/agents.
- **Tray.io**: Traditional iPaaS with an embedded product; public connector count not clearly stated in official docs. Tray provides an on-prem agent that runs on your infrastructure to connect to private systems. See: https://developer.tray.ai/openapi/trayapi/tag/connectors/ and https://docs.tray.ai/tray-uac/connecting-to-on-premise-systems/on-prem-agent/getting-started/.
- **Cyclr**: Embedded iPaaS; marketing page claims 600+ applications supported. See: https://cyclr.com/application-connectors.
- **Pandium**: Embedded iPaaS emphasizing code-first integrations; marketing lists 200+ connectors; docs note \"210\" connectors. Pandium emphasizes managed infrastructure for running integrations, with no self-host option mentioned. See: https://www.pandium.com/connectors and https://docs.pandium.com/connectors/connectors-101.
- **Ampersand**: Code-first embedded integration platform; open-source connectors library on GitHub. Connector count not publicly stated. Ampersand docs describe the Ampersand server as a managed service; connectors library is open source. See: https://github.com/amp-labs/connectors and https://docs.withampersand.com/.
- **Nango**: Code-first integration infrastructure with auth, sync, webhooks, and managed execution. Open-source core uses the Elastic License. Connector counts vary across pages: 500+ to 700+ depending on page (catalog and homepage). Nango offers a limited free self-hosted option plus an enterprise self-hosted edition. See: https://github.com/NangoHQ/nango, https://nango.dev/, https://www.nango.dev/api-integrations, and https://nango.dev/docs/guides/platform/self-hosting.
- **Frigg**: Open-source framework for building customer-facing integrations that runs in your cloud; provides an API module library, but no published connector count. See: https://lefthook.com/frigg/ and https://docs.friggframework.org/.

---

## Traditional iPaaS / workflow automation

| Product | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [n8n](https://n8n.io/) | Open source + commercial hosting | Traditional iPaaS | 1,000+ apps | Yes (self-hosted editions) | Visual workflow automation, self-hostable. |
| [Pipedream](https://pipedream.com/) | Commercial (free tier) | Traditional iPaaS | \"Thousands of apps\" | No (publicly stated no self-host option) | Code-first workflows with triggers/actions and webhook support. |
| [Workato](https://www.workato.com/) | Commercial | Traditional iPaaS | 1,000+ connectors | Partial (on-prem agent for private systems) | Enterprise automation, recipe-based workflows. |
| [Tray.io](https://tray.io/) | Commercial | Traditional iPaaS | Not publicly stated | Partial (on-prem agent for private systems) | Visual workflow builder with strong API tooling. |
| [Apache Camel](https://camel.apache.org/) | Open Source (Apache-2.0) | Integration framework; adjacent to traditional iPaaS via Camel K / Karavan | 370 non-core components (plus core components and Kamelets) | Yes (library/runtime you operate; Camel K runs on Kubernetes) | Enterprise Integration Patterns, route DSLs, components, Kamelets, and low-code/Kubernetes tooling. |

### Traditional iPaaS product notes

- **n8n**: OSS workflow automation with a very large app catalog; their integrations pages note 1,000+ apps. Docs explicitly support self-hosted editions. See: https://n8n.io/integrations/ and https://docs.n8n.io/choose-n8n/.
- **Pipedream**: Developer-first automation platform with triggers/actions and code steps; docs describe integrations as \"thousands of apps.\" Pipedream staff state there is no self-host option at this time. See: https://pipedream.com/docs/apps, https://pipedream.com/docs/workflows/building-workflows/triggers/, and https://pipedream.com/community/t/how-can-i-self-host-pipedream-on-my-development-machine-and-ec2-instance/4978.
- **Workato** and **Tray.io**: included above; widely used enterprise iPaaS tools with large connector libraries and visual builders.
- **Apache Camel**: Long-running open-source integration framework based on Enterprise Integration Patterns. Camel Core is a small embeddable Java library with DSLs for routes, URI-addressed endpoints, data formats, and a large component catalog; the current component reference lists 370 non-core components. It is not an embedded SaaS integration product by itself: teams operate it inside their own applications or infrastructure. Related projects make it more platform-like: Camel K runs Camel integrations natively on Kubernetes, Karavan provides a low-code UI for designing/configuring routes with Kamelets and components, and Kamelets package source/sink connector snippets behind simpler interfaces. Compared with AuthProxy, Camel is much broader for routing, transformation, and message mediation, but does not primarily focus on customer-facing connection lifecycle, tenant-scoped OAuth/API-key credential storage, or authenticating proxy behavior. See: https://camel.apache.org/manual/faq/what-is-camel.html, https://camel.apache.org/components/4.18.x/index.html, https://camel.apache.org/docs/, and https://camel.apache.org/.

---

## Webhook / event gateways

Webhook gateways are a distinct, narrower integration-infrastructure category.
They sit between an event producer and one or more consumers, adding durable
ingress or delivery, retries, rate control, routing, replay, signing, and
operational visibility. Unlike an iPaaS, they generally do not provide a large
catalog of authenticated API actions or own application workflows.

| Product | Direction | Packaging / license | Self-hosting | Notes |
| --- | --- | --- | --- | --- |
| [Hookdeck](https://hookdeck.com/) | Inbound and outbound | Commercial Event Gateway; Apache-2.0 Outpost runtime | Outbound: yes; inbound: no public self-host option | Event Gateway receives, queues, filters, transforms, routes, observes, and replays external events. Outpost provides multi-tenant outbound webhooks and event destinations. |
| [Svix](https://www.svix.com/) | Primarily outbound | Commercial hosting + open-source server (MIT) | Yes | Webhooks-as-a-service focused on delivery, signing, retries, monitoring, and an application portal. |
| [Convoy](https://www.getconvoy.io/) | Inbound and outbound | Commercial + source-available server (Elastic License 2.0) | Yes | Webhook gateway with routing, persistence, retries, rate limiting, circuit breaking, and an operator dashboard. |
| [Hook0](https://www.hook0.com/) | Outbound | Commercial hosting + source-available server (SSPL-1.0) | Yes | Multi-tenant webhook sending with subscriptions, signatures, retries, delivery logs, and replay. |

### Webhook gateway product notes

- **Hookdeck** spans both sides of the boundary. Its managed Event Gateway is an inbound proxy and asynchronous API gateway for accepting third-party events before they reach an application, with filtering, transformations, rate control, observability, and replay. Hookdeck Outpost handles outbound delivery to customer-managed webhooks, queues, brokers, and event buses. Outpost is Apache-2.0 and can be self-hosted; Hookdeck does not publish a self-hosted edition of the inbound Event Gateway. See: https://hookdeck.com/, https://hookdeck.com/docs, and https://github.com/hookdeck/outpost.
- **Svix** is the closest permissively licensed outbound alternative. Its hosted service and MIT-licensed self-hosted server accept one publish call and handle endpoint management, signing, retries, and delivery observability. Svix's consumer tooling also helps recipients verify and debug messages, but the core product is outbound delivery rather than a general inbound proxy. See: https://www.svix.com/ and https://github.com/svix/svix-webhooks.
- **Convoy** is a bidirectional, self-hostable gateway that can ingest provider webhooks and deliver application events to customer endpoints. Its repository is publicly available under Elastic License 2.0, which permits self-hosting but is source-available rather than OSI open source. See: https://github.com/frain-dev/convoy and https://www.getconvoy.io/core-gateway.
- **Hook0** focuses on providing outbound webhooks to a SaaS product's users. It is self-hostable and publishes its server and UI source under SSPL-1.0; that license is source-available rather than an OSI-approved open-source license. See: https://www.hook0.com/ and https://github.com/hook0/hook0.

For AuthProxy, these systems are complementary more often than substitutes. A
host application can use AuthProxy to manage a tenant's authenticated calls to
a provider and place a webhook gateway at its event boundary. The overlap is
limited to adjacent concerns such as tenant isolation, request observability,
and proxying; webhook gateways do not generally own OAuth connection setup,
token refresh, or arbitrary authenticated access to a provider's native API.

---

## API, AI, and MCP gateways

“MCP gateway” is not one product category with a settled scope. Current
products use the term for at least three different boundaries:

- A **protocol and traffic gateway** fronts MCP servers, converts APIs into MCP
  tools, and applies authentication, access policy, rate limits, and telemetry.
  Kong follows this model.
- A **tool aggregation and execution gateway** connects to MCP servers,
  controls which tools a model can discover or execute, and can expose the
  combined tools as another MCP server. Bifrost follows this model alongside
  its LLM gateway.
- An **identity-aware access proxy** enrolls existing MCP servers and governs
  which humans or agents can reach them, with RBAC and audit trails. Teleport
  follows this model.

These are worth tracking because they can sit next to AuthProxy in an agent
architecture, and some are beginning to overlap with credential lifecycle.
They do not all provide customer-facing API integrations.

| Product | Primary gateway role | API / tool coverage | Credential model | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [Composio](https://composio.dev) | Agent tool integration | 900-1000+ toolkits (500+ apps) | Managed application auth | Not stated in docs | Managed auth, triggers, tools, and MCP servers. |
| [Metorial](https://metorial.com/) | MCP integration platform | 600+ MCP servers | Managed integration auth | Yes (open source; self-hostable) | MCP server catalog, deployment, and observability. |
| [Pica](https://picaos.com) | Auth and tool-action gateway | 200+ integrations | Managed AuthKit connections | No public self-host option | AuthKit plus a passthrough API with 25k+ actions. |
| [Airweave](https://airweave.ai/) | Agent data-ingestion gateway | 50+ data sources | Source connection credentials | Yes (open source; self-host or hosted) | Unified retrieval layer for agents. |
| [LiteLLM](https://www.litellm.ai/) | LLM inference gateway | 100+ model providers | Centrally managed provider credentials | Yes (OSS self-host; cloud option) | Model routing, auth, quotas, and spend controls; not an MCP tool gateway. |
| [Bifrost](https://docs.getbifrost.ai/) | LLM inference + MCP tool gateway | 20+ model providers; arbitrary MCP servers | Provider keys; shared or per-user MCP OAuth | Yes (open source; Apache-2.0) | Acts as an MCP client and server, filters tools, and gates execution or autonomous agent mode. |
| [Kong](https://konghq.com/) | API, AI, and MCP traffic gateway | User-managed APIs and MCP servers | Gateway auth; OAuth or credential pass-through for MCP | Yes (Apache-2.0 core; commercial editions) | Proxies APIs and MCP servers, converts REST operations into tools, aggregates tools, and applies traffic policy. MCP proxy features require AI Gateway Enterprise. |
| [Teleport](https://goteleport.com/use-cases/secure-model-context-protocol/) | Identity-aware infrastructure and MCP access proxy | Enrolled MCP servers and infrastructure resources | Short-lived user/workload identity; optional signed JWT to upstream | Yes (AGPL-3.0 source; restricted Community binaries; commercial editions) | RBAC/ABAC, JIT access, per-tool audit events, and session recording for existing MCP servers. |
| [Agent Vault](https://github.com/Infisical/agent-vault) | Credential proxy for AI agents | Any HTTPS service (no connector catalog) | User-registered credentials injected at the network layer | Yes (open source; MIT) | Keeps credentials out of agent processes and constrains network access. |

### API, AI, and MCP gateway product notes

- **Composio**: Managed auth and tool execution for AI agents. Docs mention 1000+ toolkits; the toolkits catalog shows ~900 toolkits; other pages reference 500+ apps. No self-host option is documented. See: https://docs.composio.dev/, https://docs.composio.dev/toolkits, and https://docs.composio.dev/toolkits/introduction.
- **Metorial**: Open-source MCP integration platform; GitHub repo states it is open source and self-hostable, with a hosted option available. See: https://github.com/metorial/metorial.
- **Pica**: Integration infrastructure for SaaS + AI; AuthKit handles OAuth/token refresh and multi-tenant auth. Docs require a Pica account and API key, implying hosted service. See: https://docs.picaos.com/authkit/setup and https://docs.picaos.com/authkit.
- **Airweave**: Open-source context retrieval layer with 50+ prebuilt connectors; focuses on ingestion and unified search for agents. Site FAQ says it is open source and can be self-hosted or used via hosted platform. See: https://airweave.ai/ and https://docs.airweave.ai/.
- **LiteLLM**: Open-source LLM gateway supporting 100+ provider integrations, with spend tracking and routing. OSS page highlights self-hosting with no data sent to LiteLLM servers; docs show running the proxy via Docker or CLI. See: https://www.litellm.ai/oss and https://docs.litellm.ai/.
- **Bifrost**: Apache-2.0 AI gateway with OpenAI-compatible APIs and routing across 20+ model providers. Its MCP subsystem connects to STDIO, HTTP, or SSE servers, exposes aggregated tools through an MCP Gateway URL, filters tools per request, client, or virtual key, and separates tool suggestions from explicit execution by default. It also supports shared OAuth with automatic refresh and per-user OAuth for upstream MCP servers. This overlaps AuthProxy's token lifecycle for MCP-native integrations, but Bifrost is centered on model requests and MCP tool execution rather than embedded setup for arbitrary native APIs. See: https://github.com/maximhq/bifrost, https://docs.getbifrost.ai/mcp/overview, and https://docs.getbifrost.ai/mcp/connecting-to-servers.
- **Kong**: General-purpose API gateway available as an Apache-2.0 core, commercial self-managed editions, and the Konnect managed control plane. Kong centralizes routing and plugins for authentication, authorization, rate limiting, transformations, and observability across APIs. Its enterprise AI MCP Proxy can front existing MCP servers, convert OpenAPI-described REST operations into MCP tools, aggregate tool sets, and apply per-tool ACLs and standard Kong policies. This is a traffic and protocol control plane, not a tenant connection lifecycle: upstream APIs, MCP servers, identities, and credentials must still be provisioned. See: https://github.com/Kong/kong, https://developer.konghq.com/mcp/, and https://developer.konghq.com/plugins/ai-mcp-proxy/.
- **Teleport**: Infrastructure identity platform that treats MCP servers as protected resources alongside databases, Kubernetes clusters, applications, and other infrastructure. Teleport enrolls existing STDIO, SSE, or streamable-HTTP MCP servers, authenticates users and workloads, applies RBAC/ABAC and JIT access, and emits per-tool audit events and session recordings. It can pass Teleport-signed JWT identity to an upstream MCP server, but it does not create tools, translate REST APIs into MCP, or manage each end user's third-party OAuth connection. MCP enrollment, proxying, identity controls, and per-tool audit are listed for Community and Enterprise editions. The repository source is AGPL-3.0, while distributed Community binaries use a commercial license with company-size and revenue restrictions. See: https://goteleport.com/use-cases/secure-model-context-protocol/, https://goteleport.com/docs/enroll-resources/mcp-access/, and https://goteleport.com/docs/feature-matrix/.
- **Agent Vault**: Open-source HTTP credential proxy by Infisical, purpose-built for AI agents. Agents get a scoped session and a local `HTTPS_PROXY`; Agent Vault injects the credential at the network layer so credentials are never returned to the agent. Works with any HTTP-speaking agent (Claude Code, Cursor, Codex, custom Python/TypeScript, sandboxed processes) and any HTTPS API — there is no prebuilt connector catalog; you register your own services and credentials. Ships as a binary, Docker image, or from source; MIT-licensed with a separate `ee/` directory for enterprise features. Offers a container-sandbox mode (iptables-locked egress through the proxy) and an SDK for orchestrating sandboxed agents (Docker/Daytona/E2B). See: https://github.com/Infisical/agent-vault, https://docs.agent-vault.dev, and https://infisical.com/blog/agent-vault-the-open-source-credential-proxy-and-vault-for-agents.

For AuthProxy, the most direct MCP-gateway overlap is Bifrost's per-user OAuth
and token refresh for upstream MCP servers. Kong overlaps at the authenticated
proxy and policy layer, while Teleport overlaps in identity, authorization, and
audit. AuthProxy remains distinct when the product needs an embeddable
connection UI, versioned connector definitions, health and lifecycle
management, and unrestricted forwarding to each provider's native API. An MCP
gateway could consume tools backed by AuthProxy connections, or AuthProxy could
sit behind Kong or Teleport when broader traffic or infrastructure policy is
required.

---

## Open-source frameworks / building blocks

| Project | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [Frigg](https://friggframework.org/) | Open Source | Embedded framework | Not publicly stated | Yes (runs in your cloud) | Serverless framework + API modules library. |
| [Ampersand Connectors](https://github.com/amp-labs/connectors) | Open Source | Embedded building blocks | Not publicly stated | Yes (library you run yourself) | OSS connector library used by Ampersand. |
| [Hookdeck Outpost](https://github.com/hookdeck/outpost) | Open Source (Apache-2.0) | Outbound event-delivery infrastructure | N/A (destination types, not API connectors) | Yes | Multi-tenant webhooks and event destinations with retries, fan-out, observability, and a user portal. |
| [Svix Webhooks](https://github.com/svix/svix-webhooks) | Open Source (MIT) | Outbound webhook-delivery infrastructure | N/A (webhook endpoints, not API connectors) | Yes | Self-hostable webhook sending server with retries, signatures, and delivery controls. |
| [Apache Camel](https://camel.apache.org/) | Open Source (Apache-2.0) | Integration framework / building block | 370 non-core components (plus core components and Kamelets) | Yes (library/runtime you operate) | EIP-based routing and mediation framework with route DSLs, components, data formats, Camel K, and Karavan. |

### Framework notes

- **Frigg**: OSS framework for teams that want to own infrastructure and build embedded integrations; provides a modular API library and serverless architecture, and runs in your cloud. See: https://lefthook.com/frigg/ and https://docs.friggframework.org/.
- **Ampersand Connectors**: OSS connector library used by Ampersand; useful for teams building their own integration infrastructure. See: https://github.com/amp-labs/connectors.
- **Hookdeck Outpost**: Apache-2.0 outbound event-delivery runtime. It supports multi-tenant webhooks plus destinations such as SQS, Kafka, RabbitMQ, EventBridge, and Pub/Sub, and includes retries, fan-out, OpenTelemetry, and an end-user portal. See: https://github.com/hookdeck/outpost.
- **Svix Webhooks**: MIT-licensed server behind Svix's outbound webhook service. It is the other permissively licensed, self-hostable alternative in this comparison. See: https://github.com/svix/svix-webhooks.
- **Apache Camel**: OSS integration building block for teams that want full control over routes, transports, and deployment. It is strongest when the problem is message routing, mediation, protocol bridging, transformation, or running integration logic inside Java/Spring Boot/Quarkus/Kubernetes environments. It is less directly comparable to embedded iPaaS products because it does not provide a hosted multi-tenant connection UI, unified API model, or credential lifecycle out of the box. See: https://camel.apache.org/manual/faq/what-is-camel.html and https://camel.apache.org/docs/.

---

## Quick Comparison (High-Level)

| Product | Primary Use Case | Code vs UI | Eventing/Webhooks | Connector Definition | Self-hosting |
| --- | --- | --- | --- | --- | --- |
| **AuthProxy** | Embedded connection lifecycle and authenticating proxy | Code-first with embedded Marketplace and Admin UIs | Not a workflow or event platform | Declarative, versioned definitions maintained by the adopting team | Yes |
| Merge | Unified API for B2B SaaS data | API-first (code) | Webhooks supported | Vendor-defined connectors maintained by Merge | Yes (paid option) |
| Kombo | Unified HR/ATS/LMS/Payroll | API-first (code) | Webhooks supported | Vendor-defined connectors maintained by Kombo | No public self-host option |
| Paragon | Embedded integrations for SaaS | Hybrid (SDK + UI) | Webhooks + workflow triggers | Prebuilt + custom connector builder | Yes (managed or unmanaged on-prem) |
| Prismatic | Embedded iPaaS | Hybrid | Webhooks + workflows | Prebuilt + SDK | Partial (on-prem agent) |
| Workato | Traditional enterprise iPaaS | UI-heavy | Triggers + actions | Prebuilt + custom connectors | Partial (on-prem agent) |
| Tray.io | Traditional + embedded iPaaS | UI-heavy | Triggers + actions | Prebuilt + custom connector SDK | Partial (on-prem agent) |
| Cyclr | Embedded iPaaS | UI-heavy | Triggers + actions | Prebuilt + custom connector tools | Yes (self-hosted private cloud) |
| Pandium | Embedded iPaaS (code-first) | Code-first | Webhooks supported | Auth-focused connectors + code integrations | No public self-host option |
| Ampersand | Embedded iPaaS (code-first) | Code-first | Subscribe to events | Declarative YAML + OSS connectors | No public self-host option (platform) |
| Nango | Integration infrastructure | Code-first | Webhooks + syncs | Prebuilt auth + custom integrations | Yes (limited free self-host; enterprise self-host) |
| n8n | Workflow automation | UI-heavy + code nodes | Webhooks + triggers | Community + core nodes | Yes (self-hosted editions) |
| Pipedream | Workflow automation | Code-first + UI | Webhooks + triggers | App actions + custom code | No (publicly stated) |
| Hookdeck | Inbound event gateway + outbound delivery | Hybrid (API + UI + CLI) | Core product | Sources, routing connections, destinations, and delivery policies | Outbound Outpost only (Apache-2.0) |
| Svix | Outbound webhook delivery | API-first + application portal | Core product | Applications, endpoints, event types, and delivery policies | Yes (MIT server) |
| Convoy | Bidirectional webhook gateway | API + operator UI | Core product | Sources, subscriptions, endpoints, and delivery policies | Yes (Elastic License 2.0) |
| Hook0 | Outbound webhook delivery | API + UI | Core product | Applications, event types, subscriptions, and endpoints | Yes (SSPL-1.0) |
| Composio | AI agent tool access | Code-first | Triggers supported | Toolkits + MCP servers | Not stated in docs |
| Metorial | MCP integration platform | Code-first | N/A (agent tool calls) | MCP servers (hosted or OSS) | Yes (open source; self-hostable) |
| Pica | Auth + actions for AI & SaaS | Code-first + embeddable UI | Webhooks supported | AuthKit + Passthrough API | No public self-host option |
| Airweave | Agent data ingestion | Code-first | Sync + retrieval | Connectors for data sources | Yes (open source; self-host or hosted) |
| LiteLLM | LLM gateway | Code-first | N/A | Provider integrations | Yes (OSS self-host; cloud option) |
| Bifrost | LLM and MCP tool gateway | Code-first + management UI | MCP client/server and tool execution | Model providers + upstream MCP servers | Yes (Apache-2.0) |
| Kong | API, AI, and MCP traffic gateway | API/declarative config + management UI | Proxies, converts, and aggregates MCP tools | User-managed APIs, services, and MCP servers | Yes (Apache-2.0 core; MCP proxy is Enterprise) |
| Teleport | Identity-aware infrastructure and MCP access | CLI/config + management UI | Proxies and audits MCP tool access | Enrolled MCP servers and infrastructure resources | Yes (AGPL-3.0 source; restricted Community binaries; commercial editions) |
| Agent Vault | Credential brokerage for AI agents | Code-first (CLI + SDK) | N/A (network-layer proxy) | User-registered services + credentials; no prebuilt connectors | Yes (OSS MIT; binary or Docker) |
| Apache Camel | Routing, mediation, and protocol integration framework | Code-first DSLs + Karavan low-code tooling | Routes, timers, polling, messaging components, Kamelets | Components, route DSLs, Kamelets | Yes (OSS library/runtime; Camel K on Kubernetes) |
