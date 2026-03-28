# Related Products

This is a list of related products in the API Integration space and (eventually) how AuthProxy compares to them.

The landscape splits into a few overlapping domains:

- **Unified APIs**: Normalize a vertical (e.g., HRIS/ATS/Accounting) behind a single data model and endpoint set.
- **Embedded iPaaS**: Integration infrastructure designed to be embedded into your SaaS product for customer-facing integrations.
- **Traditional iPaaS / Workflow Automation**: Internal automation platforms with large connector catalogs and visual builders.
- **AI / Agent Integration Gateways**: Tool-calling, MCP servers, or AI gateways that broker auth + actions to many services.
- **Open-source Frameworks / Building Blocks**: Libraries or frameworks for teams that want full control.

Below are summaries and comparison tables for the projects in this repo's list. Connector counts are taken from the vendors' public catalogs or docs when available; when not available, it's noted explicitly.

---

**Unified APIs**

| Product | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [Merge](https://www.merge.dev/) | Commercial | Primarily embedded (customer-facing) | 220+ integrations | Yes (paid option; details not public) | Unified APIs across HRIS, ATS, CRM, Accounting, Ticketing, etc. | 
| [Kombo](https://www.kombo.dev/) | Commercial | Embedded | 250+ integrations | No public self-host option; regional SaaS (EU/US) | Unified API focused on HRIS/ATS/LMS/Payroll. |

**Unified API Product Notes**

- **Merge**: Provides unified APIs across multiple verticals plus a hosted platform that manages auth, maintenance, and monitoring. Connector count published in their integrations catalog. Supports webhooks and standard API patterns in docs. Code-first via REST APIs, with SDKs and platform tooling. Pricing indicates a self-hosted option available for purchase. See: https://www.merge.dev/integrations, https://docs.merge.dev/, and https://www.merge.dev/pricing.
- **Kombo**: Unified API focused on HR/ATS/LMS/Payroll. Connector counts published in their integrations catalog and category pages. Built for embedding into SaaS products. Docs describe EU/US regional hosting, with no self-host option mentioned. See: https://www.kombo.dev/integrations and https://docs.kombo.dev/data-residency-compliance.

---

**Embedded iPaaS (Customer-Facing Integrations)**

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

**Embedded iPaaS Product Notes**

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

**Traditional iPaaS / Workflow Automation**

| Product | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [n8n](https://n8n.io/) | Open source + commercial hosting | Traditional iPaaS | 1,000+ apps | Yes (self-hosted editions) | Visual workflow automation, self-hostable. |
| [Pipedream](https://pipedream.com/) | Commercial (free tier) | Traditional iPaaS | \"Thousands of apps\" | No (publicly stated no self-host option) | Code-first workflows with triggers/actions and webhook support. |
| [Workato](https://www.workato.com/) | Commercial | Traditional iPaaS | 1,000+ connectors | Partial (on-prem agent for private systems) | Enterprise automation, recipe-based workflows. |
| [Tray.io](https://tray.io/) | Commercial | Traditional iPaaS | Not publicly stated | Partial (on-prem agent for private systems) | Visual workflow builder with strong API tooling. |

**Traditional iPaaS Product Notes**

- **n8n**: OSS workflow automation with a very large app catalog; their integrations pages note 1,000+ apps. Docs explicitly support self-hosted editions. See: https://n8n.io/integrations/ and https://docs.n8n.io/choose-n8n/.
- **Pipedream**: Developer-first automation platform with triggers/actions and code steps; docs describe integrations as \"thousands of apps.\" Pipedream staff state there is no self-host option at this time. See: https://pipedream.com/docs/apps, https://pipedream.com/docs/workflows/building-workflows/triggers/, and https://pipedream.com/community/t/how-can-i-self-host-pipedream-on-my-development-machine-and-ec2-instance/4978.
- **Workato** and **Tray.io**: included above; widely used enterprise iPaaS tools with large connector libraries and visual builders.

---

**AI / Agent Integration Gateways**

| Product | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [Composio](https://composio.dev) | Commercial | Embedded for AI agents | 900-1000+ toolkits (500+ apps) | Not stated in docs | Managed auth, triggers, MCP servers. |
| [Metorial](https://metorial.com/) | OSS + commercial hosting | Embedded for AI agents | 600+ MCP servers | Yes (open source; self-hostable) | MCP-native integration platform and observability. |
| [Pica](https://picaos.com) | Commercial | Embedded for AI + SaaS | 200+ integrations | No public self-host option; SaaS with API key | AuthKit + Passthrough API with 25k+ actions. |
| [Airweave](https://airweave.ai/) | Open source + hosted | Embedded for AI (data ingestion) | 50+ data sources | Yes (open source; self-host or hosted) | Unified retrieval layer for agents. |
| [LiteLLM](https://www.litellm.ai/) | Open source + commercial | AI gateway | 100+ providers | Yes (OSS self-host; cloud option) | LLM gateway with auth, quotas, and routing. |
| [Bifrost](https://docs.getbifrost.ai/) | Open source + commercial | AI gateway | 20+ providers | Yes (self-hostable gateway) | OpenAI-compatible gateway with governance and routing. |

**AI / Agent Product Notes**

- **Composio**: Managed auth and tool execution for AI agents. Docs mention 1000+ toolkits; the toolkits catalog shows ~900 toolkits; other pages reference 500+ apps. No self-host option is documented. See: https://docs.composio.dev/, https://docs.composio.dev/toolkits, and https://docs.composio.dev/toolkits/introduction.
- **Metorial**: Open-source MCP integration platform; GitHub repo states it is open source and self-hostable, with a hosted option available. See: https://github.com/metorial/metorial.
- **Pica**: Integration infrastructure for SaaS + AI; AuthKit handles OAuth/token refresh and multi-tenant auth. Docs require a Pica account and API key, implying hosted service. See: https://docs.picaos.com/authkit/setup and https://docs.picaos.com/authkit.
- **Airweave**: Open-source context retrieval layer with 50+ prebuilt connectors; focuses on ingestion and unified search for agents. Site FAQ says it is open source and can be self-hosted or used via hosted platform. See: https://airweave.ai/ and https://docs.airweave.ai/.
- **LiteLLM**: Open-source LLM gateway supporting 100+ provider integrations, with spend tracking and routing. OSS page highlights self-hosting with no data sent to LiteLLM servers; docs show running the proxy via Docker or CLI. See: https://www.litellm.ai/oss and https://docs.litellm.ai/.
- **Bifrost**: Open-source AI gateway with 20+ providers and OpenAI-compatible APIs; supports routing, failover, and governance. GitHub repo is Apache-2.0 and docs show local deployment via Docker or NPX. See: https://github.com/maximhq/bifrost and https://docs.getbifrost.ai/.

---

**Open-Source Frameworks / Building Blocks**

| Project | Commercial / OSS | Traditional vs Embedded | Connector Count | Self-hosting | Notes |
| --- | --- | --- | --- | --- | --- |
| [Frigg](https://friggframework.org/) | Open Source | Embedded framework | Not publicly stated | Yes (runs in your cloud) | Serverless framework + API modules library. |
| [Ampersand Connectors](https://github.com/amp-labs/connectors) | Open Source | Embedded building blocks | Not publicly stated | Yes (library you run yourself) | OSS connector library used by Ampersand. |

**Framework Notes**

- **Frigg**: OSS framework for teams that want to own infrastructure and build embedded integrations; provides a modular API library and serverless architecture, and runs in your cloud. See: https://lefthook.com/frigg/ and https://docs.friggframework.org/.
- **Ampersand Connectors**: OSS connector library used by Ampersand; useful for teams building their own integration infrastructure. See: https://github.com/amp-labs/connectors.

---

## Quick Comparison (High-Level)

| Product | Primary Use Case | Code vs UI | Eventing/Webhooks | Connector Definition | Self-hosting |
| --- | --- | --- | --- | --- | --- |
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
| Composio | AI agent tool access | Code-first | Triggers supported | Toolkits + MCP servers | Not stated in docs |
| Metorial | MCP integration platform | Code-first | N/A (agent tool calls) | MCP servers (hosted or OSS) | Yes (open source; self-hostable) |
| Pica | Auth + actions for AI & SaaS | Code-first + embeddable UI | Webhooks supported | AuthKit + Passthrough API | No public self-host option |
| Airweave | Agent data ingestion | Code-first | Sync + retrieval | Connectors for data sources | Yes (open source; self-host or hosted) |
| LiteLLM | LLM gateway | Code-first | N/A | Provider integrations | Yes (OSS self-host; cloud option) |
| Bifrost | LLM gateway | Code-first | N/A | Provider integrations | Yes (self-hostable gateway) |
