# Connector Resource Schema

This package separates the versioned `Connector` resource from its provider-only `ConnectorDefinition`. Resource identity, generation, labels, annotations, release intent, and observed state belong in the resource envelope. Only `spec.definition` is encrypted in the connector-definition database column and included in the semantic definition hash.

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Connector
metadata:
  id: cxr_google0123456789
  name: google-drive
  namespace: root
  generation: 3
  labels:
    category: productivity
  annotations:
    example.com/owner: integrations
spec:
  release:
    desiredState: primary
  definition:
    displayName: Google Drive
    logo:
      publicUrl: https://example.com/google-drive.png
    description: Connect Google Drive
    auth:
      type: no-auth
status:
  release:
    state: primary
```

Configuration may use `metadata.id` or `metadata.name` to identify a connector and may set `metadata.generation` explicitly. It must omit `status`, which is server-owned. API request/response wrappers that have not yet migrated to the resource envelope remain in `internal/schema/api`.

Connector-author setup-flow guidance lives in [`docs/src/content/docs/integration/connector-setup-flow.md`](../../../../docs/src/content/docs/integration/connector-setup-flow.md). Shared predicate behavior for setup steps, OAuth scopes, and probes lives in [`docs/src/content/docs/integration/connector-predicates.md`](../../../../docs/src/content/docs/integration/connector-predicates.md).
