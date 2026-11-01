# Config Schema

This package defines AuthProxy's YAML/JSON configuration file syntax. It can compose common primitives, auth permissions, and resource definitions that are valid in configuration files.

Runtime resource models that are also exposed through REST APIs should live under `internal/schema/resources/...` and be referenced from config instead of duplicated here.

`tools.invocation` defines optional server limits for Tool execution. This package
validates supplied values; core translates them into runtime policy and supplies
defaults. It does not import executors or duplicate their runtime types. See the
[configuration reference](../../../docs/src/content/docs/reference/configuration.md#tool-invocation-limits)
for examples, defaults, and the current invocation boundary.
