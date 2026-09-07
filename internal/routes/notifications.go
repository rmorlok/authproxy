package routes

import (
	"fmt"
	"maps"
	"net/http"

	"github.com/gin-gonic/gin"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type NotificationsRoutes struct {
	auth auth.A
	core coreIface.C
}

type NotificationJson = schemaapi.NotificationJson
type ListNotificationsResponseJson = schemaapi.ListNotificationsResponseJson
type OpenAPIListNotificationsResponseJson = schemaapiopenapi.ListNotificationsResponseJson
type OpenAPINotificationViewActionJson = schemaapiopenapi.NotificationViewActionJson
type OpenAPINotificationBatchViewActionJson = schemaapiopenapi.NotificationBatchViewActionJson

type ListNotificationsRequestQuery struct {
	LimitVal      *uint64 `form:"limit"`
	IncludeViewed *bool   `form:"includeViewed"`
	State         *string `form:"state"`
	NamespaceVal  *string `form:"namespace"`
	LabelSelector *string `form:"labelSelector"`
}

// @Summary		List notifications
// @Description	List active actor-visible notifications
// @Tags			notifications
// @Produce		json
// @Param			limit			query		int		false	"Maximum number of notifications to return"
// @Param			includeViewed	query		bool	false	"Include notifications the actor has already viewed"
// @Param			state			query		string	false	"Notification state; defaults to active"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			labelSelector	query		string	false	"Filter by denormalized resource label selector"
// @Success		200				{object}	OpenAPIListNotificationsResponseJson
// @Failure		400				{object}	ErrorResponse
// @Failure		401				{object}	ErrorResponse
// @Failure		500				{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/notifications [get]
func (r *NotificationsRoutes) list(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	ra := auth.MustGetAuthFromGinContext(gctx)

	var req ListNotificationsRequestQuery
	if err := gctx.ShouldBindQuery(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest(err.Error(), httperr.WithInternalErr(err)))
		return
	}

	limit := uint64(100)
	if req.LimitVal != nil {
		if *req.LimitVal == 0 {
			apgin.WriteError(gctx, nil, httperr.BadRequest("limit must be a positive integer"))
			return
		}
		limit = *req.LimitVal
	}

	state := database.NotificationStateActive
	if req.State != nil {
		state = database.NotificationState(*req.State)
		if !database.IsValidNotificationState(state) {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid state"))
			return
		}
	}

	includeViewed := false
	if req.IncludeViewed != nil {
		includeViewed = *req.IncludeViewed
	}

	var namespaceMatchers []string
	if req.NamespaceVal != nil {
		if err := namespace.ValidateMatcher(*req.NamespaceVal); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid namespace", httperr.WithInternalErr(err)))
			return
		}
		namespaceMatchers = []string{*req.NamespaceVal}
	}
	if req.LabelSelector != nil {
		if _, err := database.ParseLabelSelector(*req.LabelSelector); err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequest("invalid label_selector", httperr.WithInternalErr(err)))
			return
		}
	}

	notifications, err := r.core.ListActorNotifications(ctx, ra, database.ListNotificationsOptions{
		States:            []database.NotificationState{state},
		NamespaceMatchers: namespaceMatchers,
		LabelSelector:     req.LabelSelector,
		Limit:             limit,
		IncludeViewed:     includeViewed,
	})
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}

	items := make([]NotificationJson, 0, len(notifications))
	for _, n := range notifications {
		item, err := notificationToJSON(n)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.InternalServerError(httperr.WithInternalErr(err)))
			return
		}
		items = append(items, item)
	}

	apgin.APIJSON(gctx, http.StatusOK, schemaapi.NewListNotificationsResponseJson(items, ""))
}

// @Summary		Mark notifications viewed
// @Description	Mark multiple notifications viewed for the authenticated actor
// @Tags			notifications
// @Accept			json
// @Produce		json
// @Param			request	body	OpenAPINotificationBatchViewActionJson	true	"Notification batch view action"
// @Success		200	{object}	OpenAPINotificationBatchViewActionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/notifications/_viewed [post]
func (r *NotificationsRoutes) markViewedBatch(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	ra := auth.MustGetAuthFromGinContext(gctx)

	var req schemaapi.NotificationBatchViewAction
	if err := apgin.BindActionJSON(
		gctx,
		&req,
		schemaapi.NotificationBatchViewActionKind,
	); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		return
	}

	ids := make([]apid.ID, 0, len(req.Metadata.Targets))
	for _, target := range req.Metadata.Targets {
		id, err := apid.Parse(target.ID)
		if err != nil {
			apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
			return
		}
		ids = append(ids, id)
	}
	if err := r.core.MarkActorNotificationsViewed(ctx, ra, ids); err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}
	response := schemaapi.NewNotificationBatchViewResponse(req.Metadata.Targets)
	if err := apgin.RenderActionJSON(
		gctx,
		http.StatusOK,
		&response,
		schemaapi.NotificationBatchViewActionKind,
	); err != nil {
		apgin.WriteErr(gctx, nil, err)
	}
}

// @Summary		Mark notification viewed
// @Description	Mark a notification viewed for the authenticated actor
// @Tags			notifications
// @Accept		json
// @Produce		json
// @Param			id	path	string	true	"Notification ID"
// @Param			request	body	OpenAPINotificationViewActionJson	true	"Notification view action"
// @Success		200	{object}	OpenAPINotificationViewActionJson
// @Failure		400	{object}	ErrorResponse
// @Failure		401	{object}	ErrorResponse
// @Failure		403	{object}	ErrorResponse
// @Failure		404	{object}	ErrorResponse
// @Failure		500	{object}	ErrorResponse
// @Security		BearerAuth
// @Router			/notifications/{id}/_viewed [post]
func (r *NotificationsRoutes) markViewed(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	ra := auth.MustGetAuthFromGinContext(gctx)

	id, err := apid.Parse(gctx.Param("id"))
	if err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid id format", httperr.WithInternalErr(err)))
		return
	}
	if id == apid.Nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("id is required"))
		return
	}
	if err := id.ValidatePrefix(apid.PrefixNotification); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequest("invalid notification id", httperr.WithInternalErr(err)))
		return
	}
	var req schemaapi.NotificationViewAction
	if err := apgin.BindActionJSON(gctx, &req, schemaapi.NotificationViewActionKind); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		return
	}
	if req.Metadata.Target.ID != id.String() {
		apgin.WriteError(gctx, nil, httperr.BadRequest("metadata.target.id does not match the notification path"))
		return
	}

	if err := r.core.MarkActorNotificationViewed(ctx, ra, id); err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}
	response := schemaapi.NewNotificationViewResponse(req.Metadata.Target)
	if err := apgin.RenderActionJSON(gctx, http.StatusOK, &response, schemaapi.NotificationViewActionKind); err != nil {
		apgin.WriteErr(gctx, nil, err)
	}
}

func notificationToJSON(actorNotification coreIface.ActorNotification) (NotificationJson, error) {
	n := actorNotification.Notification
	resourceKind, err := resourceKindForStoredType(n.ResourceType)
	if err != nil {
		return NotificationJson{}, fmt.Errorf("build notification resource reference: %w", err)
	}
	var action *schemaapi.NotificationActionStatusJson
	if actorNotification.CanAction && n.ActionUrl != nil {
		action = &schemaapi.NotificationActionStatusJson{URL: *n.ActionUrl}
	}
	contextData := map[string]any(n.Metadata)
	if len(contextData) == 0 {
		contextData = nil
	}
	createdAt := n.CreatedAt
	updatedAt := n.UpdatedAt
	return NotificationJson{
		TypeMeta: meta.NewTypeMeta(schemaapi.NotificationKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:        n.Id.String(),
			Namespace: n.Namespace,
			Labels:    maps.Clone(map[string]string(n.Labels)),
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		}),
		Spec: schemaapi.NotificationSpecJson{
			Key:   n.Key,
			Level: schemaapi.NotificationLevel(n.Level),
			ResourceRef: meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       resourceKind,
				ID:         n.ResourceId.String(),
			},
			Title:   n.Title,
			Message: n.Message,
			Context: contextData,
		},
		Status: schemaapi.NotificationStatusJson{
			State:      schemaapi.NotificationState(n.State),
			Viewed:     actorNotification.Viewed,
			Action:     action,
			ResolvedAt: n.ResolvedAt,
		},
	}, nil
}

func (r *NotificationsRoutes) Register(g gin.IRouter) {
	g.GET("/notifications", r.auth.Required(), r.list)
	g.POST("/notifications/_viewed", r.auth.Required(), r.markViewedBatch)
	g.POST("/notifications/:id/_viewed", r.auth.Required(), r.markViewed)
}

func NewNotificationsRoutes(authService auth.A, core coreIface.C) *NotificationsRoutes {
	return &NotificationsRoutes{
		auth: authService,
		core: core,
	}
}
