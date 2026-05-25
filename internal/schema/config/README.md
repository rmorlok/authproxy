# Config Schema

This package defines AuthProxy's YAML/JSON configuration file syntax. It can compose common primitives, auth permissions, and resource definitions that are valid in configuration files.

Runtime resource models that are also exposed through REST APIs should live under `internal/schema/resources/...` and be referenced from config instead of duplicated here.
