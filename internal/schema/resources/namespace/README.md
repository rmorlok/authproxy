# Namespace Resource Schema

This package owns namespace path and matcher validation plus constants for namespace hierarchy behavior.

Use this package for namespace resource semantics. Auth permissions reference namespace matchers, but namespace validation itself is not auth-specific.

API request and response DTOs for namespace routes live in `internal/schema/api`. Keep this package focused on reusable namespace primitives: paths, matchers, hierarchy helpers, and the JSON Schema definitions other packages can reference.
