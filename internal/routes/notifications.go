package routes

import (
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
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type NotificationsRoutes struct {
	auth auth.A
	core coreIface.C
}

type NotificationJson = schemaapi.NotificationJson
type ListNotificationsResponseJson = schemaapi.ListNotificationsResponseJson
type MarkNotificationsViewedRequestJson = schemaapi.MarkNotificationsViewedRequestJson
type OpenAPIListNotificationsResponseJson = schemaapiopenapi.ListNotificationsResponseJson

type ListNotificationsRequestQuery struct {
	LimitVal      *uint64 `form:"limit"`
	IncludeViewed *bool   `form:"include_viewed"`
	State         *string `form:"state"`
	NamespaceVal  *string `form:"namespace"`
	LabelSelector *string `form:"label_selector"`
}

// @Summary		List notifications
// @Description	List active actor-visible notifications
// @Tags			notifications
// @Produce		json
// @Param			limit			query		int		false	"Maximum number of notifications to return"
// @Param			include_viewed	query		bool	false	"Include notifications the actor has already viewed"
// @Param			state			query		string	false	"Notification state; defaults to active"
// @Param			namespace		query		string	false	"Filter by namespace"
// @Param			label_selector	query		string	false	"Filter by denormalized resource label selector"
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
		items = append(items, notificationToJSON(n))
	}

	apgin.APIJSON(gctx, http.StatusOK, ListNotificationsResponseJson{Items: items})
}

// @Summary		Mark notifications viewed
// @Description	Mark multiple notifications viewed for the authenticated actor
// @Tags			notifications
// @Accept			json
// @Produce		json
// @Param			request	body	MarkNotificationsViewedRequestJson	true	"Notification IDs"
// @Success		204
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

	var req MarkNotificationsViewedRequestJson
	if err := gctx.ShouldBindBodyWithJSON(&req); err != nil {
		apgin.WriteError(gctx, nil, httperr.BadRequestErr(err))
		return
	}

	if err := r.core.MarkActorNotificationsViewed(ctx, ra, req.Ids); err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}
	gctx.Status(http.StatusNoContent)
}

// @Summary		Mark notification viewed
// @Description	Mark a notification viewed for the authenticated actor
// @Tags			notifications
// @Produce		json
// @Param			id	path	string	true	"Notification ID"
// @Success		204
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

	if err := r.core.MarkActorNotificationViewed(ctx, ra, id); err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}
	gctx.Status(http.StatusNoContent)
}

func notificationToJSON(actorNotification coreIface.ActorNotification) NotificationJson {
	n := actorNotification.Notification
	actionURL := ""
	if actorNotification.CanAction && n.ActionUrl != nil {
		actionURL = *n.ActionUrl
	}
	metadata := map[string]any(n.Metadata)
	if len(metadata) == 0 {
		metadata = nil
	}
	return NotificationJson{
		Id:           n.Id,
		Key:          n.Key,
		Level:        schemaapi.NotificationLevel(n.Level),
		State:        schemaapi.NotificationState(n.State),
		ResourceType: n.ResourceType,
		ResourceId:   n.ResourceId,
		Namespace:    n.Namespace,
		Title:        n.Title,
		Message:      n.Message,
		ActionUrl:    actionURL,
		CanAction:    actorNotification.CanAction,
		Viewed:       actorNotification.Viewed,
		Metadata:     metadata,
		CreatedAt:    n.CreatedAt,
		UpdatedAt:    n.UpdatedAt,
		ResolvedAt:   n.ResolvedAt,
	}
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
