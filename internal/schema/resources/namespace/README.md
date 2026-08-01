# Namespace Resource Schema

This package owns namespace path and matcher validation plus constants for namespace hierarchy behavior.

Use this package for namespace resource semantics. Auth permissions reference namespace matchers, but namespace validation itself is not auth-specific.

API request and response DTOs for namespace routes live in `internal/schema/api`. Keep this package focused on reusable namespace primitives: paths, matchers, hierarchy helpers, and the JSON Schema definitions other packages can reference.

A namespace's read-only resource name is set automatically to the final segment
of its path when the namespace is created. For example, `root.prod.billing` has
the name `billing`. Namespace paths are immutable, so namespace names cannot be
changed after creation.
