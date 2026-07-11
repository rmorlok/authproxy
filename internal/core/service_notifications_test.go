package core

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/database"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/stretchr/testify/require"
)

func TestListActorNotificationsCachesActorFilteredResult(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := mockDb.NewMockDB(ctrl)
	s := newNotificationTestService(t, db)

	actorID := apid.New(apid.PrefixActor)
	connectionID := apid.New(apid.PrefixConnection)
	notification := notificationTestNotification(connectionID)
	ra := notificationTestAuth(actorID, connectionID)
	viewedAt := time.Unix(100, 0).UTC()

	db.EXPECT().
		ListNotifications(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, opts database.ListNotificationsOptions) ([]database.Notification, error) {
			require.Equal(t, actorID, opts.ActorId)
			return []database.Notification{notification}, nil
		}).
		Times(1)
	db.EXPECT().
		NotificationViewedMap(gomock.Any(), actorID, []apid.ID{notification.Id}).
		Return(map[apid.ID]time.Time{notification.Id: viewedAt}, nil).
		Times(1)

	first, err := s.ListActorNotifications(ctx, ra, notificationListTestOptions())
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.True(t, first[0].Viewed)
	require.True(t, first[0].CanAction)

	second, err := s.ListActorNotifications(ctx, ra, notificationListTestOptions())
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestListActorNotificationsCacheKeyIncludesRequestPermissions(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := mockDb.NewMockDB(ctrl)
	s := newNotificationTestService(t, db)

	actorID := apid.New(apid.PrefixActor)
	connectionID := apid.New(apid.PrefixConnection)
	notification := notificationTestNotification(connectionID)
	actor := notificationTestActor(actorID, connectionID)
	fullAuth := authcore.NewAuthenticatedRequestAuth(actor)
	getOnlyAuth := authcore.NewAuthenticatedRequestAuthWithPermissions(
		actor,
		aschema.PermissionsSingleWithResourceIds("root", "connections", "get", connectionID.String()),
	)

	db.EXPECT().
		ListNotifications(gomock.Any(), gomock.Any()).
		Return([]database.Notification{notification}, nil).
		Times(2)
	db.EXPECT().
		NotificationViewedMap(gomock.Any(), actorID, []apid.ID{notification.Id}).
		Return(map[apid.ID]time.Time{}, nil).
		Times(2)

	fullResult, err := s.ListActorNotifications(ctx, fullAuth, notificationListTestOptions())
	require.NoError(t, err)
	require.Len(t, fullResult, 1)
	require.True(t, fullResult[0].CanAction)

	restrictedResult, err := s.ListActorNotifications(ctx, getOnlyAuth, notificationListTestOptions())
	require.NoError(t, err)
	require.Len(t, restrictedResult, 1)
	require.False(t, restrictedResult[0].CanAction)
}

func TestNotificationCacheInvalidatesOnMarkViewed(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := mockDb.NewMockDB(ctrl)
	s := newNotificationTestService(t, db)

	actorID := apid.New(apid.PrefixActor)
	connectionID := apid.New(apid.PrefixConnection)
	notification := notificationTestNotification(connectionID)
	ra := notificationTestAuth(actorID, connectionID)
	viewedAt := time.Unix(200, 0).UTC()

	gomock.InOrder(
		db.EXPECT().
			ListNotifications(gomock.Any(), gomock.Any()).
			Return([]database.Notification{notification}, nil),
		db.EXPECT().
			NotificationViewedMap(gomock.Any(), actorID, []apid.ID{notification.Id}).
			Return(map[apid.ID]time.Time{}, nil),
		db.EXPECT().
			GetNotification(gomock.Any(), notification.Id).
			Return(&notification, nil),
		db.EXPECT().
			MarkNotificationsViewed(gomock.Any(), []apid.ID{notification.Id}, actorID).
			Return(nil),
		db.EXPECT().
			ListNotifications(gomock.Any(), gomock.Any()).
			Return([]database.Notification{notification}, nil),
		db.EXPECT().
			NotificationViewedMap(gomock.Any(), actorID, []apid.ID{notification.Id}).
			Return(map[apid.ID]time.Time{notification.Id: viewedAt}, nil),
	)

	before, err := s.ListActorNotifications(ctx, ra, notificationListTestOptions())
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.False(t, before[0].Viewed)

	require.NoError(t, s.MarkActorNotificationViewed(ctx, ra, notification.Id))

	after, err := s.ListActorNotifications(ctx, ra, notificationListTestOptions())
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.True(t, after[0].Viewed)
}

func TestNotificationCacheInvalidatesOnMarkNotificationsViewed(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := mockDb.NewMockDB(ctrl)
	s := newNotificationTestService(t, db)

	actorID := apid.New(apid.PrefixActor)
	connectionID := apid.New(apid.PrefixConnection)
	first := notificationTestNotification(connectionID)
	second := notificationTestNotification(connectionID)
	second.Key = "connection:" + connectionID.String() + ":setup_required"
	ra := notificationTestAuth(actorID, connectionID)

	gomock.InOrder(
		db.EXPECT().
			GetNotification(gomock.Any(), first.Id).
			Return(&first, nil),
		db.EXPECT().
			GetNotification(gomock.Any(), second.Id).
			Return(&second, nil),
		db.EXPECT().
			MarkNotificationsViewed(gomock.Any(), []apid.ID{first.Id, second.Id}, actorID).
			Return(nil),
	)

	require.NoError(t, s.MarkActorNotificationsViewed(ctx, ra, []apid.ID{first.Id, second.Id, first.Id}))
	require.Equal(t, "1", s.r.Get(ctx, notificationCacheVersionKey).Val())
}

func TestNotificationWriteHelpersBumpCacheVersion(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	db := mockDb.NewMockDB(ctrl)
	s := newNotificationTestService(t, db)

	connectionID := apid.New(apid.PrefixConnection)
	notification := notificationTestNotification(connectionID)
	upsert := database.NotificationUpsert{
		Key:               notification.Key,
		Level:             notification.Level,
		ResourceType:      notification.ResourceType,
		ResourceId:        notification.ResourceId,
		Namespace:         notification.Namespace,
		Labels:            notification.Labels,
		Title:             notification.Title,
		Message:           notification.Message,
		ActionUrl:         notification.ActionUrl,
		ViewPermissions:   notification.ViewPermissions,
		ActionPermissions: notification.ActionPermissions,
		Metadata:          notification.Metadata,
	}

	db.EXPECT().
		UpsertNotification(gomock.Any(), upsert).
		Return(&notification, nil)
	db.EXPECT().
		ResolveNotificationsForResourceKeys(gomock.Any(), "connection", connectionID, []string{notification.Key}).
		Return(nil)

	_, err := s.upsertNotification(ctx, upsert)
	require.NoError(t, err)
	require.Equal(t, "1", s.r.Get(ctx, notificationCacheVersionKey).Val())

	require.NoError(t, s.resolveNotificationsForResourceKeys(ctx, "connection", connectionID, []string{notification.Key}))
	require.Equal(t, "2", s.r.Get(ctx, notificationCacheVersionKey).Val())
}

func newNotificationTestService(t *testing.T, db *mockDb.MockDB) *service {
	t.Helper()

	_, r := apredis.MustApplyTestConfig(nil)
	require.NoError(t, r.FlushDB(context.Background()).Err())
	return &service{
		db:     db,
		r:      r,
		logger: aplog.NewNoopLogger(),
	}
}

func notificationListTestOptions() database.ListNotificationsOptions {
	return database.ListNotificationsOptions{
		States: []database.NotificationState{database.NotificationStateActive},
		Limit:  100,
	}
}

func notificationTestAuth(actorID apid.ID, connectionID apid.ID) *authcore.RequestAuth {
	return authcore.NewAuthenticatedRequestAuth(notificationTestActor(actorID, connectionID))
}

func notificationTestActor(actorID apid.ID, connectionID apid.ID) *authcore.Actor {
	return &authcore.Actor{
		Id:         actorID,
		ExternalId: "actor",
		Namespace:  "root",
		Permissions: []aschema.Permission{{
			Namespace:   "root",
			Resources:   []string{"connections"},
			ResourceIds: []string{connectionID.String()},
			Verbs:       []string{"get", "update"},
		}},
	}
}

func notificationTestNotification(connectionID apid.ID) database.Notification {
	now := time.Unix(50, 0).UTC()
	actionURL := "/connections/" + connectionID.String() + "?action=reauth"
	return database.Notification{
		Id:           apid.New(apid.PrefixNotification),
		Key:          "connection:" + connectionID.String() + ":auth_required",
		Level:        database.NotificationLevelWarning,
		State:        database.NotificationStateActive,
		ResourceType: "connection",
		ResourceId:   connectionID,
		Namespace:    "root",
		Labels:       database.Labels{"env": "test"},
		Title:        "Authentication required",
		Message:      "Reconnect this connection.",
		ActionUrl:    &actionURL,
		ViewPermissions: aschema.PermissionsSingleWithResourceIds(
			"root",
			"connections",
			"get",
			connectionID.String(),
		),
		ActionPermissions: aschema.PermissionsSingleWithResourceIds(
			"root",
			"connections",
			"update",
			connectionID.String(),
		),
		Metadata:  database.NotificationMetadata{"reason": "test"},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
