---
title: Design notes
---

These pages preserve implementation plans, migrations, and known product gaps.
They are contributor context, not stable user-facing contracts.

- [OAuth provider identity changes](/development/design/oauth-provider-identity/) — current gap
  and target behavior for enforcing a stable provider account during
  reauthentication.
- [Unified key-model migration](/development/design/key-model-migration/) — migration history,
  data model, provider behavior, and implementation sequencing for logical
  keys and data-encryption keys.

Stable behavior belongs in the public concept, integration, operations, or
security guides. Move a design note into those sections only after the behavior
has shipped and the page has been rewritten as user documentation.
