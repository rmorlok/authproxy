# API Schema

This package contains endpoint-specific API request and response DTOs plus 
shared API transports. These types describe wire contracts at AuthProxy route 
boundaries that are not already canonical resource contracts.

Canonical resources and their resource-specific patches live in 
`internal/schema/resources/...`; routes use those types directly when the entire
body is the resource or patch. Do not add aliases here merely to rename a 
resource for a particular endpoint. API models may compose shared primitives 
from `internal/schema/common` and resource models from 
`internal/schema/resources/...`. Resource packages must not import this package.

Auxiliary route DTOs also belong here: session initiation, request-events list
envelopes, task status and monitoring responses, and shared label/annotation 
key-value bodies. Keep route packages focused on binding, validation, 
authorization, and conversion to/from service-layer types.

Notifications and search make their non-resource semantics explicit here.
`NotificationJson` is a read-only, resource-shaped projection of a durable
notification row because its `status.viewed` and `status.action` values are
calculated for the authenticated actor; it is not a client-managed desired
resource. Search items are heterogeneous summaries, not partial resources.
`SearchResultList` therefore carries typed `resourceRef` values and puts
truncation/incompleteness observations in projection metadata. Notification
view mutations use typed action contracts.

OpenAPI-only generator adapters live under `internal/schema/api/openapi`. Keep
those adapters thin: they may compose API DTOs and canonical resources for 
documentation, but no runtime contract or behavior may depend on them.

Route handlers should import endpoint-specific DTOs from this package and 
canonical resources from their resource package, or convert those contracts to 
runtime/core/database models. Do not add new endpoint-specific `*RequestJson` 
or `*ResponseJson` structs under `internal/routes`; preflight rejects that so 
contract ownership stays clear.
