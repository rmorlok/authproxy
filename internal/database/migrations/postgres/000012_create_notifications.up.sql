create table notifications
(
    id                 text primary key,
    key                text not null,
    level              text not null,
    state              text not null,
    resource_type      text not null,
    resource_id        text not null,
    namespace          text not null,
    title              text not null,
    message            text not null,
    action_url         text,
    view_permissions   jsonb,
    action_permissions jsonb,
    source             text,
    metadata           jsonb,
    resolved_at        timestamptz,
    created_at         timestamptz not null,
    updated_at         timestamptz not null,
    deleted_at         timestamptz
);

create unique index idx_notifications_key_active
    on notifications (key)
    where deleted_at is null;

create index idx_notifications_resource
    on notifications (resource_type, resource_id, state)
    where deleted_at is null;

create index idx_notifications_namespace_state
    on notifications (namespace, state, created_at desc)
    where deleted_at is null;

create table notification_views
(
    notification_id text not null,
    actor_id        text not null,
    viewed_at       timestamptz not null,
    created_at      timestamptz not null,
    updated_at      timestamptz not null,
    primary key (notification_id, actor_id)
);

create index idx_notification_views_actor
    on notification_views (actor_id, viewed_at desc);
