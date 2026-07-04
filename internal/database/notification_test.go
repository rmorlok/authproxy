package database

import (
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
)

func TestNotifications(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	now := time.Date(2026, time.July, 3, 12, 0, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	connID := apid.New(apid.PrefixConnection)
	source := "connection_required_action"
	actionURL := "/connections/" + connID.String() + "?action=reauth"
	notification, err := db.UpsertNotification(ctx, NotificationUpsert{
		Key:          "connection:" + connID.String() + ":auth_required",
		Level:        NotificationLevelWarning,
		ResourceType: "connection",
		ResourceId:   connID,
		Namespace:    "root",
		Title:        "Reauth required",
		Message:      "Please reconnect this connection.",
		ActionUrl:    &actionURL,
		ViewPermissions: aschema.PermissionsSingleWithResourceIds(
			"root",
			"connections",
			"get",
			connID.String(),
		),
		ActionPermissions: aschema.PermissionsSingleWithResourceIds(
			"root",
			"connections",
			"update",
			connID.String(),
		),
		Source: &source,
		Metadata: map[string]any{
			"target_version": float64(2),
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, notification.Id)
	require.Equal(t, NotificationStateActive, notification.State)

	updated, err := db.UpsertNotification(ctx, NotificationUpsert{
		Key:          notification.Key,
		Level:        NotificationLevelError,
		ResourceType: "connection",
		ResourceId:   connID,
		Namespace:    "root",
		Title:        "Still required",
		Message:      "Please reconnect this connection again.",
		Source:       &source,
	})
	require.NoError(t, err)
	require.Equal(t, notification.Id, updated.Id)
	require.Equal(t, NotificationLevelError, updated.Level)
	require.Equal(t, "Still required", updated.Title)

	items, err := db.ListNotifications(ctx, ListNotificationsOptions{
		States: []NotificationState{NotificationStateActive},
		Limit:  10,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, updated.Id, items[0].Id)

	actorID := apid.New(apid.PrefixActor)
	require.NoError(t, db.MarkNotificationViewed(ctx, updated.Id, actorID))
	viewed, err := db.NotificationViewedMap(ctx, actorID, []apid.ID{updated.Id})
	require.NoError(t, err)
	require.Contains(t, viewed, updated.Id)

	require.NoError(t, db.ResolveNotificationsForResource(ctx, "connection", connID, source, nil))
	resolved, err := db.GetNotification(ctx, updated.Id)
	require.NoError(t, err)
	require.Equal(t, NotificationStateResolved, resolved.State)
	require.NotNil(t, resolved.ResolvedAt)
}

func TestListNotificationsFiltersNamespaceAndLabels(t *testing.T) {
	_, db := MustApplyBlankTestDbConfig(t, nil)
	ctx := apctx.NewBuilderBackground().
		WithClock(clock.NewFakeClock(time.Date(2026, time.July, 3, 12, 0, 0, 0, time.UTC))).
		Build()

	rootConnID := apid.New(apid.PrefixConnection)
	childConnID := apid.New(apid.PrefixConnection)
	otherConnID := apid.New(apid.PrefixConnection)

	rootNotification, err := db.UpsertNotification(ctx, NotificationUpsert{
		Key:          "notification:root",
		Level:        NotificationLevelInfo,
		ResourceType: "connection",
		ResourceId:   rootConnID,
		Namespace:    "root",
		Labels: map[string]string{
			"env":  "prod",
			"team": "payments",
		},
		Title:   "Root notification",
		Message: "Root message",
	})
	require.NoError(t, err)
	require.Equal(t, Labels{"env": "prod", "team": "payments"}, rootNotification.Labels)

	childNotification, err := db.UpsertNotification(ctx, NotificationUpsert{
		Key:          "notification:child",
		Level:        NotificationLevelInfo,
		ResourceType: "connection",
		ResourceId:   childConnID,
		Namespace:    "root.child",
		Labels: map[string]string{
			"env":  "prod",
			"team": "sales",
		},
		Title:   "Child notification",
		Message: "Child message",
	})
	require.NoError(t, err)

	_, err = db.UpsertNotification(ctx, NotificationUpsert{
		Key:          "notification:other",
		Level:        NotificationLevelInfo,
		ResourceType: "connection",
		ResourceId:   otherConnID,
		Namespace:    "root.other",
		Labels: map[string]string{
			"env":  "dev",
			"team": "payments",
		},
		Title:   "Other notification",
		Message: "Other message",
	})
	require.NoError(t, err)

	items, err := db.ListNotifications(ctx, ListNotificationsOptions{
		NamespaceMatchers: []string{"root.child.**"},
		Limit:             10,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, childNotification.Id, items[0].Id)

	selector := "env=prod,team=payments"
	items, err = db.ListNotifications(ctx, ListNotificationsOptions{
		NamespaceMatchers: []string{"root.**"},
		LabelSelector:     &selector,
		Limit:             10,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, rootNotification.Id, items[0].Id)

	missingSelector := "missing"
	items, err = db.ListNotifications(ctx, ListNotificationsOptions{
		LabelSelector: &missingSelector,
		Limit:         10,
	})
	require.NoError(t, err)
	require.Empty(t, items)

	_, err = db.ListNotifications(ctx, ListNotificationsOptions{
		NamespaceMatchers: []string{"!!invalid!!"},
	})
	require.Error(t, err)

	invalidSelector := "bad key=value"
	_, err = db.ListNotifications(ctx, ListNotificationsOptions{
		LabelSelector: &invalidSelector,
	})
	require.Error(t, err)
}
