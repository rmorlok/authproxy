package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	authservice "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	coreiface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

type notificationsRouteTestCore struct {
	coreiface.C
	list      func(context.Context, *authcore.RequestAuth, database.ListNotificationsOptions) ([]coreiface.ActorNotification, error)
	mark      func(context.Context, *authcore.RequestAuth, apid.ID) error
	markBatch func(context.Context, *authcore.RequestAuth, []apid.ID) error
}

func (c notificationsRouteTestCore) ListActorNotifications(
	ctx context.Context,
	ra *authcore.RequestAuth,
	opts database.ListNotificationsOptions,
) ([]coreiface.ActorNotification, error) {
	return c.list(ctx, ra, opts)
}

func (c notificationsRouteTestCore) MarkActorNotificationViewed(
	ctx context.Context,
	ra *authcore.RequestAuth,
	id apid.ID,
) error {
	return c.mark(ctx, ra, id)
}

func (c notificationsRouteTestCore) MarkActorNotificationsViewed(
	ctx context.Context,
	ra *authcore.RequestAuth,
	ids []apid.ID,
) error {
	return c.markBatch(ctx, ra, ids)
}

type notificationsRouteSetup struct {
	router   *gin.Engine
	authUtil *authservice.AuthTestUtil
}

func setupNotificationsRoute(t *testing.T, core coreiface.C) notificationsRouteSetup {
	t.Helper()
	cfg, db := database.MustApplyBlankTestDbConfig(t, config.FromRoot(&sconfig.Root{}))
	_, auth, authUtil := authservice.TestAuthServiceWithDb(sconfig.ServiceIdAdminApi, cfg, db)
	router := apgin.ForTest(nil)
	NewNotificationsRoutes(auth, core).Register(router)
	return notificationsRouteSetup{router: router, authUtil: authUtil}
}

func signedNotificationRequest(
	t *testing.T,
	setup notificationsRouteSetup,
	method, url string,
	body any,
) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	req, err := setup.authUtil.NewSignedRequestForActorExternalId(
		method,
		url,
		reader,
		"root",
		"notification-caller",
		aschema.AllPermissions(),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestNotificationToJSONBuildsActorProjection(t *testing.T) {
	now := time.Now().UTC()
	actionURL := "/connections/cxn_test0000000000001?action=reauth"
	item, err := notificationToJSON(coreiface.ActorNotification{
		Notification: database.Notification{
			Id:           apid.New(apid.PrefixNotification),
			Key:          "connection:reauth",
			Level:        database.NotificationLevelWarning,
			State:        database.NotificationStateActive,
			ResourceType: string(database.SearchResourceTypeConnection),
			ResourceId:   apid.MustParse("cxn_test0000000000001"),
			Namespace:    "root.acme",
			Labels:       database.Labels{"team": "payments"},
			Title:        "Reauthenticate",
			Message:      "Credentials must be refreshed.",
			ActionUrl:    &actionURL,
			Metadata:     database.NotificationMetadata{"targetVersion": 2},
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		Viewed:    true,
		CanAction: true,
	})
	require.NoError(t, err)
	require.Equal(t, meta.NewTypeMeta(schemaapi.NotificationKind), item.TypeMeta)
	require.Equal(t, "root.acme", item.Metadata.Namespace)
	require.Equal(t, map[string]string{"team": "payments"}, item.Metadata.Labels)
	require.Equal(t, connectionschema.ConnectionKind, item.Spec.ResourceRef.Kind)
	require.Equal(t, "cxn_test0000000000001", item.Spec.ResourceRef.ID)
	require.Equal(t, map[string]any{"targetVersion": 2}, item.Spec.Context)
	require.True(t, item.Status.Viewed)
	require.Equal(t, actionURL, item.Status.Action.URL)
}

func TestNotificationToJSONOmitsUnauthorizedAction(t *testing.T) {
	actionURL := "/connections/cxn_test0000000000001?action=reauth"
	item, err := notificationToJSON(coreiface.ActorNotification{
		Notification: database.Notification{
			Id:           apid.New(apid.PrefixNotification),
			ResourceType: string(database.SearchResourceTypeConnection),
			ResourceId:   apid.MustParse("cxn_test0000000000001"),
			ActionUrl:    &actionURL,
		},
	})
	require.NoError(t, err)
	require.Nil(t, item.Status.Action)
}

func TestNotificationToJSONRejectsUnsupportedResourceType(t *testing.T) {
	_, err := notificationToJSON(coreiface.ActorNotification{Notification: database.Notification{
		ResourceType: "request_event",
	}})
	require.ErrorContains(t, err, "unsupported resource type")
}

func TestNotificationsRouteReturnsResourceList(t *testing.T) {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	notificationID := apid.New(apid.PrefixNotification)
	core := notificationsRouteTestCore{
		list: func(_ context.Context, _ *authcore.RequestAuth, opts database.ListNotificationsOptions) ([]coreiface.ActorNotification, error) {
			require.Equal(t, uint64(100), opts.Limit)
			return []coreiface.ActorNotification{{Notification: database.Notification{
				Id:           notificationID,
				Key:          "connection:reauth",
				Level:        database.NotificationLevelWarning,
				State:        database.NotificationStateActive,
				ResourceType: string(database.SearchResourceTypeConnection),
				ResourceId:   apid.MustParse("cxn_test0000000000001"),
				Namespace:    "root.acme",
				Title:        "Reauthenticate",
				Message:      "Credentials must be refreshed.",
				CreatedAt:    now,
				UpdatedAt:    now,
			}}}, nil
		},
	}
	setup := setupNotificationsRoute(t, core)
	w := httptest.NewRecorder()
	setup.router.ServeHTTP(w, signedNotificationRequest(t, setup, http.MethodGet, "/notifications", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response schemaapi.ListNotificationsResponseJson
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, meta.NewTypeMeta("NotificationList"), response.TypeMeta)
	require.Len(t, response.Items, 1)
	require.Equal(t, notificationID.String(), response.Items[0].Metadata.ID)
	require.Equal(t, connectionschema.ConnectionKind, response.Items[0].Spec.ResourceRef.Kind)
}

func TestNotificationsRouteViewActions(t *testing.T) {
	first := apid.New(apid.PrefixNotification)
	second := apid.New(apid.PrefixNotification)
	var marked apid.ID
	var markedBatch []apid.ID
	core := notificationsRouteTestCore{
		mark: func(_ context.Context, _ *authcore.RequestAuth, id apid.ID) error {
			marked = id
			return nil
		},
		markBatch: func(_ context.Context, _ *authcore.RequestAuth, ids []apid.ID) error {
			markedBatch = append([]apid.ID(nil), ids...)
			return nil
		},
	}
	setup := setupNotificationsRoute(t, core)

	t.Run("single", func(t *testing.T) {
		request := schemaapi.NewNotificationViewRequest(schemaapi.NewNotificationReference(first))
		w := httptest.NewRecorder()
		setup.router.ServeHTTP(w, signedNotificationRequest(
			t,
			setup,
			http.MethodPost,
			"/notifications/"+first.String()+"/_viewed",
			request,
		))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, first, marked)
		var response schemaapi.NotificationViewAction
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.NoError(t, response.ValidateResponse(schemaapi.NotificationViewActionKind))
	})

	t.Run("target must match path", func(t *testing.T) {
		marked = apid.Nil
		request := schemaapi.NewNotificationViewRequest(schemaapi.NewNotificationReference(second))
		w := httptest.NewRecorder()
		setup.router.ServeHTTP(w, signedNotificationRequest(
			t,
			setup,
			http.MethodPost,
			"/notifications/"+first.String()+"/_viewed",
			request,
		))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		require.Equal(t, apid.Nil, marked)
	})

	t.Run("batch", func(t *testing.T) {
		targets := []meta.ObjectReference{
			schemaapi.NewNotificationReference(first),
			schemaapi.NewNotificationReference(second),
		}
		request := schemaapi.NotificationBatchViewAction{
			TypeMeta: meta.NewTypeMeta(schemaapi.NotificationBatchViewActionKind),
			Metadata: &schemaapi.NotificationBatchViewMetadata{Targets: targets},
			Spec:     &schemaapi.NotificationBatchViewSpec{},
		}
		w := httptest.NewRecorder()
		setup.router.ServeHTTP(w, signedNotificationRequest(
			t,
			setup,
			http.MethodPost,
			"/notifications/_viewed",
			request,
		))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, []apid.ID{first, second}, markedBatch)
		var response schemaapi.NotificationBatchViewAction
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		require.NoError(t, response.ValidateResponse(schemaapi.NotificationBatchViewActionKind))
	})

	t.Run("legacy ids body is rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		setup.router.ServeHTTP(w, signedNotificationRequest(
			t,
			setup,
			http.MethodPost,
			"/notifications/_viewed",
			map[string]any{"ids": []string{first.String()}},
		))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})
}
