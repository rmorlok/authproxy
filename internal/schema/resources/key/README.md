# Key Resource Schema

This package owns the canonical `authproxy.net/v1alpha1`, `kind: Key` resource,
its patch type, and the reusable key-provider contracts used by configuration,
encryption, and signing code.

Managed keys use the common resource envelope:

```yaml
apiVersion: authproxy.net/v1alpha1
kind: Key
metadata:
  id: key_example
  name: primary-key
  namespace: root
spec:
  usage: data_encryption
  materialType: symmetric
  desiredState: active
  keyData:
    value: '********'
status:
  state: active
  keyDataConfigured: true
```

`KeySpec` contains desired policy plus provider configuration. `KeyStatus` is
server-owned and reports observed lifecycle/configuration state. `KeyPatch`
uses pointer and presence-aware fields so omitted values remain unchanged,
empty metadata maps can clear labels or annotations, and `spec.keyData: null`
is rejected rather than silently clearing encrypted configuration.

Provider configuration mixes safe locators with secret fields. API handlers
must call `RedactKeyData` before rendering a managed Key. That helper removes
the ability to replay key material even when the caller has the general
`secrets/replay` permission. The `LogValue` implementations similarly exclude
provider configuration from structured logs.

The package also contains `SigningKey`, the configuration-only public/private
or shared signing-key value formerly named `Key`. It is intentionally distinct
from the managed resource. `internal/schema/config` may re-export it under the
configuration-facing name `Key`, but REST contracts use this package's managed
`Key` type directly.

Pagination remains an API transport concern in `internal/schema/api`; flat SQL
rows remain in `internal/database` and are converted at the core boundary.
