package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type NotificationLevel string

const (
	NotificationLevelInfo    NotificationLevel = "info"
	NotificationLevelWarning NotificationLevel = "warning"
	NotificationLevelError   NotificationLevel = "error"
)

const (
	// NotificationKeyAuthRequired is the condition key suffix used when a
	// connection requires the actor to re-authenticate, e.g.
	// "connection:cxn_...:auth_required".
	NotificationKeyAuthRequired = "auth_required"

	// NotificationKeySetupRequired is the condition key suffix used when a
	// connection requires additional setup, e.g.
	// "connection:cxn_...:setup_required".
	NotificationKeySetupRequired = "setup_required"
)

func IsValidNotificationLevel[T string | NotificationLevel](level T) bool {
	switch NotificationLevel(level) {
	case NotificationLevelInfo, NotificationLevelWarning, NotificationLevelError:
		return true
	default:
		return false
	}
}

type NotificationState string

const (
	NotificationStateActive   NotificationState = "active"
	NotificationStateResolved NotificationState = "resolved"
)

func IsValidNotificationState[T string | NotificationState](state T) bool {
	switch NotificationState(state) {
	case NotificationStateActive, NotificationStateResolved:
		return true
	default:
		return false
	}
}

const (
	NotificationsTable     = "notifications"
	NotificationViewsTable = "notification_views"
)

type NotificationPermissions []aschema.Permission

func (p NotificationPermissions) Value() (driver.Value, error) {
	if len(p) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (p *NotificationPermissions) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			*p = nil
			return nil
		}
		return json.Unmarshal([]byte(v), p)
	case []byte:
		if len(v) == 0 {
			*p = nil
			return nil
		}
		return json.Unmarshal(v, p)
	default:
		return fmt.Errorf("cannot convert %T to NotificationPermissions", value)
	}
}

type NotificationMetadata map[string]any

func (m NotificationMetadata) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func (m *NotificationMetadata) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			*m = nil
			return nil
		}
		return json.Unmarshal([]byte(v), m)
	case []byte:
		if len(v) == 0 {
			*m = nil
			return nil
		}
		return json.Unmarshal(v, m)
	default:
		return fmt.Errorf("cannot convert %T to NotificationMetadata", value)
	}
}

type Notification struct {
	// Id is the stable notification id, e.g. "ntf_...".
	Id apid.ID
	// Key is the deterministic dedupe key for the notification condition.
	// Reusing the same key upserts the active row instead of creating another
	// notification; for example:
	// "connection:cxn_...:auth_required".
	Key string
	// Level describes severity for presentation, such as "info" or "warning".
	Level NotificationLevel
	// State is "active" while the notification is visible and "resolved" once
	// the underlying condition no longer applies.
	State NotificationState
	// ResourceType names the resource that owns this notification, e.g.
	// "connection".
	ResourceType string
	// ResourceId is the owning resource id, e.g. a "cxn_..." connection id.
	ResourceId apid.ID
	// Namespace is the owning resource namespace used for list filtering and
	// permission checks, e.g. "root.acme".
	Namespace string
	// Labels is a denormalized snapshot of the owning resource labels so
	// notification lists can use label selectors such as "env=prod".
	Labels Labels
	// Title is the short user-facing summary shown in notification lists.
	Title string
	// Message is the longer user-facing explanation shown in detail surfaces.
	Message string
	// ActionUrl is the relative route for the suggested action, e.g.
	// "/connections/cxn_...?action=reauth"; it is only returned to actors that
	// satisfy ActionPermissions.
	ActionUrl *string
	// ViewPermissions are the stored permissions an actor must satisfy before
	// seeing the notification, usually a resource "get" permission.
	ViewPermissions NotificationPermissions
	// ActionPermissions are the stored permissions an actor must satisfy before
	// receiving ActionUrl/can_action=true, usually a resource "update"
	// permission.
	ActionPermissions NotificationPermissions
	// Source identifies the producer for group resolution and cleanup, e.g.
	// "connection_required_action".
	Source *string
	// Metadata is producer-specific structured context for debugging or UI
	// hints, e.g. {"target_version": 3, "requires_reauth": true}.
	Metadata NotificationMetadata
	// ResolvedAt is set when State transitions to resolved.
	ResolvedAt *time.Time
	// CreatedAt is when the notification row was first inserted.
	CreatedAt time.Time
	// UpdatedAt is when the notification was last changed or reactivated.
	UpdatedAt time.Time
	// DeletedAt is reserved for soft deletion; non-nil rows are excluded.
	DeletedAt *time.Time
}

func (n *Notification) cols() []string {
	return []string{
		"id",
		"key",
		"level",
		"state",
		"resource_type",
		"resource_id",
		"namespace",
		"labels",
		"title",
		"message",
		"action_url",
		"view_permissions",
		"action_permissions",
		"source",
		"metadata",
		"resolved_at",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (n *Notification) fields() []any {
	return []any{
		&n.Id,
		&n.Key,
		&n.Level,
		&n.State,
		&n.ResourceType,
		&n.ResourceId,
		&n.Namespace,
		&n.Labels,
		&n.Title,
		&n.Message,
		&n.ActionUrl,
		&n.ViewPermissions,
		&n.ActionPermissions,
		&n.Source,
		&n.Metadata,
		&n.ResolvedAt,
		&n.CreatedAt,
		&n.UpdatedAt,
		&n.DeletedAt,
	}
}

func (n *Notification) values() []any {
	return []any{
		n.Id,
		n.Key,
		n.Level,
		n.State,
		n.ResourceType,
		n.ResourceId,
		n.Namespace,
		n.Labels,
		n.Title,
		n.Message,
		n.ActionUrl,
		n.ViewPermissions,
		n.ActionPermissions,
		n.Source,
		n.Metadata,
		n.ResolvedAt,
		n.CreatedAt,
		n.UpdatedAt,
		n.DeletedAt,
	}
}

func (n *Notification) GetId() apid.ID {
	return n.Id
}

func (n *Notification) GetNamespace() string {
	return n.Namespace
}

func (n *Notification) Validate() error {
	result := &multierror.Error{}
	if n.Id == apid.Nil {
		result = multierror.Append(result, errors.New("notification id is required"))
	}
	if err := n.Id.ValidatePrefix(apid.PrefixNotification); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid notification id: %w", err))
	}
	if n.Key == "" {
		result = multierror.Append(result, errors.New("notification key is required"))
	}
	if !IsValidNotificationLevel(n.Level) {
		result = multierror.Append(result, errors.New("invalid notification level"))
	}
	if !IsValidNotificationState(n.State) {
		result = multierror.Append(result, errors.New("invalid notification state"))
	}
	if n.ResourceType == "" {
		result = multierror.Append(result, errors.New("notification resource type is required"))
	}
	if n.ResourceId == apid.Nil {
		result = multierror.Append(result, errors.New("notification resource id is required"))
	}
	if err := namespace.ValidatePath(n.Namespace); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid notification namespace: %w", err))
	}
	if err := ValidateLabels(n.Labels); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid notification labels: %w", err))
	}
	if n.Title == "" {
		result = multierror.Append(result, errors.New("notification title is required"))
	}
	if n.Message == "" {
		result = multierror.Append(result, errors.New("notification message is required"))
	}
	for i, p := range n.ViewPermissions {
		if err := p.Validate(); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid view permission %d: %w", i, err))
		}
	}
	for i, p := range n.ActionPermissions {
		if err := p.Validate(); err != nil {
			result = multierror.Append(result, fmt.Errorf("invalid action permission %d: %w", i, err))
		}
	}
	return result.ErrorOrNil()
}

type NotificationUpsert struct {
	Key               string
	Level             NotificationLevel
	ResourceType      string
	ResourceId        apid.ID
	Namespace         string
	Labels            map[string]string
	Title             string
	Message           string
	ActionUrl         *string
	ViewPermissions   []aschema.Permission
	ActionPermissions []aschema.Permission
	Source            *string
	Metadata          map[string]any
}

type ListNotificationsOptions struct {
	States            []NotificationState
	ResourceType      string
	ResourceId        apid.ID
	Source            string
	NamespaceMatchers []string
	LabelSelector     *string
	Limit             uint64
	IncludeViewed     bool
	ActorId           apid.ID
}

func (s *service) UpsertNotification(
	ctx context.Context,
	upsert NotificationUpsert,
) (*Notification, error) {
	now := apctx.GetClock(ctx).Now()
	n := &Notification{
		Id:                apid.New(apid.PrefixNotification),
		Key:               upsert.Key,
		Level:             upsert.Level,
		State:             NotificationStateActive,
		ResourceType:      upsert.ResourceType,
		ResourceId:        upsert.ResourceId,
		Namespace:         upsert.Namespace,
		Labels:            Labels(upsert.Labels),
		Title:             upsert.Title,
		Message:           upsert.Message,
		ActionUrl:         upsert.ActionUrl,
		ViewPermissions:   NotificationPermissions(upsert.ViewPermissions),
		ActionPermissions: NotificationPermissions(upsert.ActionPermissions),
		Source:            upsert.Source,
		Metadata:          NotificationMetadata(upsert.Metadata),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}

	var result *Notification
	err := s.transaction(func(tx *sql.Tx) error {
		existing, err := s.getNotificationByKeyTx(ctx, tx, n.Key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if existing == nil {
			_, err := s.sq.
				Insert(NotificationsTable).
				Columns(n.cols()...).
				Values(n.values()...).
				RunWith(tx).
				Exec()
			if err != nil {
				return err
			}
			result = n
			return nil
		}

		dbResult, err := s.sq.
			Update(NotificationsTable).
			Set("level", n.Level).
			Set("state", NotificationStateActive).
			Set("resource_type", n.ResourceType).
			Set("resource_id", n.ResourceId).
			Set("namespace", n.Namespace).
			Set("labels", n.Labels).
			Set("title", n.Title).
			Set("message", n.Message).
			Set("action_url", n.ActionUrl).
			Set("view_permissions", n.ViewPermissions).
			Set("action_permissions", n.ActionPermissions).
			Set("source", n.Source).
			Set("metadata", n.Metadata).
			Set("resolved_at", nil).
			Set("updated_at", now).
			Where(sq.Eq{"id": existing.Id, "deleted_at": nil}).
			RunWith(tx).
			Exec()
		if err != nil {
			return err
		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			return err
		}

		if affected != 1 {
			return fmt.Errorf("notification upsert updated %d rows: %w", affected, ErrViolation)
		}

		// Update from the result in the database
		existing.Level = n.Level
		existing.State = NotificationStateActive
		existing.ResourceType = n.ResourceType
		existing.ResourceId = n.ResourceId
		existing.Namespace = n.Namespace
		existing.Labels = n.Labels
		existing.Title = n.Title
		existing.Message = n.Message
		existing.ActionUrl = n.ActionUrl
		existing.ViewPermissions = n.ViewPermissions
		existing.ActionPermissions = n.ActionPermissions
		existing.Source = n.Source
		existing.Metadata = n.Metadata
		existing.ResolvedAt = nil
		existing.UpdatedAt = now
		result = existing

		return nil
	})
	return result, err
}

func (s *service) getNotificationByKeyTx(
	ctx context.Context,
	tx *sql.Tx,
	key string,
) (*Notification, error) {
	var result Notification
	err := s.sq.
		Select(result.cols()...).
		From(NotificationsTable).
		Where(sq.Eq{"key": key, "deleted_at": nil}).
		RunWith(tx).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &result, nil
}

func (s *service) GetNotification(
	ctx context.Context,
	id apid.ID,
) (*Notification, error) {
	var result Notification
	err := s.sq.
		Select(result.cols()...).
		From(NotificationsTable).
		Where(sq.Eq{"id": id, "deleted_at": nil}).
		RunWith(s.db).
		QueryRowContext(ctx).
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &result, nil
}

func (s *service) ListNotifications(
	ctx context.Context,
	opts ListNotificationsOptions,
) ([]Notification, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}
	query := s.sq.
		Select((&Notification{}).cols()...).
		From(NotificationsTable).
		Where(sq.Eq{"deleted_at": nil}).
		OrderBy("created_at desc", "id desc").
		Limit(limit)
	if len(opts.States) > 0 {
		query = query.Where(sq.Eq{"state": opts.States})
	}
	if opts.ResourceType != "" {
		query = query.Where(sq.Eq{"resource_type": opts.ResourceType})
	}
	if opts.ResourceId != apid.Nil {
		query = query.Where(sq.Eq{"resource_id": opts.ResourceId})
	}
	if opts.Source != "" {
		query = query.Where(sq.Eq{"source": opts.Source})
	}
	if len(opts.NamespaceMatchers) > 0 {
		for _, matcher := range opts.NamespaceMatchers {
			if err := namespace.ValidateMatcher(matcher); err != nil {
				return nil, err
			}
		}
		query = restrictToNamespaceMatchers(query, "namespace", opts.NamespaceMatchers)
	}
	if opts.LabelSelector != nil {
		selector, err := ParseLabelSelector(*opts.LabelSelector)
		if err != nil {
			return nil, err
		}
		query = selector.ApplyToSqlBuilderWithProvider(query, "labels", s.cfg.GetProvider())
	}
	if opts.ActorId != apid.Nil && !opts.IncludeViewed {
		query = query.
			LeftJoin(
				fmt.Sprintf(
					"%s nv ON nv.notification_id = %s.id AND nv.actor_id = ?",
					NotificationViewsTable,
					NotificationsTable,
				),
				opts.ActorId,
			).
			Where("nv.notification_id IS NULL")
	}

	rows, err := query.
		RunWith(s.db).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(n.fields()...); err != nil {
			return nil, err
		}
		results = append(results, n)
	}

	return results, rows.Err()
}

func (s *service) MarkNotificationViewed(
	ctx context.Context,
	notificationID apid.ID,
	actorID apid.ID,
) error {
	if notificationID == apid.Nil {
		return errors.New("notification id is required")
	}
	if err := notificationID.ValidatePrefix(apid.PrefixNotification); err != nil {
		return err
	}
	if actorID == apid.Nil {
		return errors.New("actor id is required")
	}
	if err := actorID.ValidatePrefix(apid.PrefixActor); err != nil {
		return err
	}

	// Check that the notification exists
	_, err := s.GetNotification(ctx, notificationID)
	if err != nil {
		return err
	}

	// Insert a viewed record or update the viewed at time if already exists
	now := apctx.GetClock(ctx).Now()
	_, err = s.sq.
		Insert(NotificationViewsTable).
		Columns("notification_id", "actor_id", "viewed_at", "created_at", "updated_at").
		Values(notificationID, actorID, now, now, now).
		Suffix("ON CONFLICT(notification_id, actor_id) DO UPDATE SET viewed_at = ?, updated_at = ?", now, now).
		RunWith(s.db).
		ExecContext(ctx)
	return err
}

// NotificationViewedMap returns a map of notification IDs to
// viewed at times for the given actor ID and notification IDs.
func (s *service) NotificationViewedMap(
	ctx context.Context,
	actorID apid.ID, // actor ID to check viewed at times for
	ids []apid.ID, // notification IDs
) (map[apid.ID]time.Time, error) {
	result := make(map[apid.ID]time.Time)

	if actorID == apid.Nil || len(ids) == 0 {
		return result, nil
	}

	rows, err := s.sq.
		Select("notification_id", "viewed_at").
		From(NotificationViewsTable).
		Where(sq.Eq{"actor_id": actorID, "notification_id": ids}).
		RunWith(s.db).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		var id apid.ID
		var viewedAt time.Time
		if err := rows.Scan(&id, &viewedAt); err != nil {
			return nil, err
		}
		result[id] = viewedAt
	}

	return result, rows.Err()
}

func (s *service) ResolveNotificationsForResource(
	ctx context.Context,
	resourceType string,
	resourceID apid.ID,
	source string,
	keepKeys []string,
) error {
	if resourceType == "" {
		return errors.New("resource type is required")
	}
	if resourceID == apid.Nil {
		return errors.New("resource id is required")
	}

	now := apctx.GetClock(ctx).Now()
	query := s.sq.
		Update(NotificationsTable).
		Set("state", NotificationStateResolved).
		Set("resolved_at", now).
		Set("updated_at", now).
		Where(sq.Eq{
			"resource_type": resourceType,
			"resource_id":   resourceID,
			"state":         NotificationStateActive,
			"deleted_at":    nil,
		})

	if source != "" {
		query = query.Where(sq.Eq{"source": source})
	}

	if len(keepKeys) > 0 {
		query = query.Where(sq.NotEq{"key": keepKeys})
	}

	_, err := query.
		RunWith(s.db).
		ExecContext(ctx)
	return err
}

func (s *service) ResolveNotificationsForResourceKeys(
	ctx context.Context,
	resourceType string,
	resourceID apid.ID,
	source string,
	keys []string,
) error {
	if len(keys) == 0 {
		return nil
	}
	if resourceType == "" {
		return errors.New("resource type is required")
	}
	if resourceID == apid.Nil {
		return errors.New("resource id is required")
	}

	now := apctx.GetClock(ctx).Now()
	query := s.sq.
		Update(NotificationsTable).
		Set("state", NotificationStateResolved).
		Set("resolved_at", now).
		Set("updated_at", now).
		Where(sq.Eq{
			"resource_type": resourceType,
			"resource_id":   resourceID,
			"key":           keys,
			"state":         NotificationStateActive,
			"deleted_at":    nil,
		})

	if source != "" {
		query = query.Where(sq.Eq{"source": source})
	}

	_, err := query.
		RunWith(s.db).
		ExecContext(ctx)
	return err
}
