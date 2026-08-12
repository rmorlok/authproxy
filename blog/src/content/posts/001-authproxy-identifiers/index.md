---
title: AuthProxy Identifiers
description: What are the different identifiers on AuthProxy resources and how are they used?
publishedAt: 2026-08-02
author: Ryan Morlok
tags:
  - announcements
draft: false
---

AuthProxy has several identifiers in the system. This post explains what they
are, what format they take, and what uniqueness constraints exist for that
identifier.

# ID

All resources in AuthProxy have an ID that is system-generated and globally
unique. IDs take the form `pre_<value>` where `pre_` is a type-dependent
prefix. Examples: `cxr_79TT3CYqAPlfYuu8` (connector),
`cxn_2kWONo9c1qidDeLe` (connection), `act_Lq5vt56QoZJ7ltYb` actor.

# Namespace

Namespace is the hierarchical path that a resource lives in. Namespaces take
the form `root.child.grandchild` where the top-level namespace must always be
named root. A examples of how namespaces are intended to be used are
`root.prod.team123.user789` and `root.dev.user123`. If the AuthProxy control
plane is used across environments, separating namespaces by environment can be
useful. Because AuthProxy uses namespaces as the layer for authorization against
resources, a common pattern is that the actual namespaces connections will be
created in are specific to entities in the host system.

# Name

Names are mutable identifiers on resources for the purpose of providing
API-defined identifiers for updates on resources defined via YAML. So while ID
is system defined as part of resource creation, name can be set as part of the
resource data and be used to query for the resource later.

Names must be unique for a particular resource type within a namespace. If not
specified, names will default to the id value for that resource. Names can be
changed after resource creation.

Namespace resource names are an exception to the rule. Their name must be the
leaf path value and cannot be changed.

Names could be thought of as standardized labels on resources where uniqueness
is enforced at the namespace level for that resource type.

# ExternalID (Actor)

`ExternalId` is an actor-specific identifier that is used to correlate an actor
with the entity in the host system that is doing the action. Usually this is a
user, so `ExternalId` would be the user id from the host system. `ExternalId`
is part of the system where actors do not need to be pre-registered prior to
calling AuthProxy. Actors will dynamically create or use existing based on
`ExternalId`.

From AuthProxy's perspective, `ExternalId` is just a string. If the host system
uses an integer identifier, then it should encode that as a string value.
`ExternalId`s must be unique within the namespace in which the actor is
defined. The same value can be used in another actor namespace, so an actor is
looked up by the pair of its namespace and `ExternalId`.

## Identifier comparison

| Dimension | System ID | Namespace | Name | Actor `ExternalId` |
| --- | --- | --- | --- | --- |
| Purpose | AuthProxy's stable reference to one resource. | Places a resource in the authorization hierarchy. | Gives a resource a human-chosen, declarative identifier. | Correlates an actor with its identity in the host system. |
| Example value | `cxn_2kWONo9c1qidDeLe` | `root.prod.team123.user789` | `google-drive-primary` | `user_123` |
| Set by | AuthProxy when the resource is created. | The control-plane or host application namespace model. | The resource definition; defaults to the system ID when omitted. | The host application. |
| Uniqueness scope | Global across AuthProxy resources. | The complete path is globally unique; each child segment is unique under its parent. | Per resource type within a namespace. | Per actor namespace; together, namespace and `ExternalId` identify the actor. |
| Mutable? | No. | No; a namespace path is its identity. | Yes, except for a namespace's leaf name. | Technically yes, but treat it as stable because it is the host-to-actor lookup key. |
| Best use | Store it when an API call or another resource must refer to this exact resource. | Model tenants, environments, teams, and other authorization boundaries. | Keep YAML-defined resources readable and addressable across deployments. | Find or create the actor associated with a host user or service. |
