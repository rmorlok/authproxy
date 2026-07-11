package routes

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	auth "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httperr"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	schemaapiopenapi "github.com/rmorlok/authproxy/internal/schema/api/openapi"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type NotificationsRoutes struct {
	auth auth.A
	db   database.DB
}

type NotificationJson = schemaapi.NotificationJson
type ListNotificationsResponseJson = schemaapi.ListNotificationsResponseJson
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

	actor := ra.MustGetActor()
	notifications, err := r.db.ListNotifications(ctx, database.ListNotificationsOptions{
		States:            []database.NotificationState{state},
		NamespaceMatchers: namespaceMatchers,
		LabelSelector:     req.LabelSelector,
		Limit:             limit,
		ActorId:           actor.GetId(),
		IncludeViewed:     includeViewed,
	})
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}

	ids := make([]apid.ID, 0, len(notifications))
	for _, n := range notifications {
		ids = append(ids, n.Id)
	}
	viewed, err := r.db.NotificationViewedMap(ctx, actor.GetId(), ids)
	if err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}

	items := make([]NotificationJson, 0, len(notifications))
	for _, n := range notifications {
		if !notificationPermissionsAllow(ra, n.ViewPermissions, n) {
			continue
		}
		item := notificationToJSON(ra, n, viewed)
		items = append(items, item)
	}

	apgin.APIJSON(gctx, http.StatusOK, ListNotificationsResponseJson{Items: items})
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

	notification, err := r.db.GetNotification(ctx, id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			apgin.WriteError(gctx, nil, httperr.NotFound("notification not found"))
		} else {
			apgin.WriteErr(gctx, nil, err)
		}
		return
	}
	if !notificationPermissionsAllow(ra, notification.ViewPermissions, *notification) {
		apgin.WriteError(gctx, nil, httperr.Forbidden("permission denied"))
		return
	}

	if err := r.db.MarkNotificationViewed(ctx, id, ra.MustGetActor().GetId()); err != nil {
		apgin.WriteErr(gctx, nil, err)
		return
	}
	gctx.Status(http.StatusNoContent)
}

func notificationToJSON(ra *authcore.RequestAuth, n database.Notification, viewed map[apid.ID]time.Time) NotificationJson {
	canAction := notificationPermissionsAllow(ra, n.ActionPermissions, n)
	actionURL := ""
	if canAction && n.ActionUrl != nil {
		actionURL = *n.ActionUrl
	}
	metadata := map[string]any(n.Metadata)
	if len(metadata) == 0 {
		metadata = nil
	}
	_, isViewed := viewed[n.Id]
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
		CanAction:    canAction,
		Viewed:       isViewed,
		Metadata:     metadata,
		CreatedAt:    n.CreatedAt,
		UpdatedAt:    n.UpdatedAt,
		ResolvedAt:   n.ResolvedAt,
	}
}

func notificationPermissionsAllow(ra *authcore.RequestAuth, permissions []aschema.Permission, n database.Notification) bool {
	if ra == nil || !ra.IsAuthenticated() || len(permissions) == 0 {
		return false
	}
	for _, p := range permissions {
		resources := p.Resources
		if len(resources) == 0 {
			resources = []string{n.ResourceType}
		}
		verbs := p.Verbs
		if len(verbs) == 0 {
			verbs = []string{"get"}
		}
		resourceIds := p.ResourceIds
		if len(resourceIds) == 0 {
			resourceIds = []string{n.ResourceId.String()}
		}
		for _, resource := range resources {
			for _, verb := range verbs {
				for _, resourceId := range resourceIds {
					if ra.Allows(n.Namespace, resource, verb, resourceId) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (r *NotificationsRoutes) Register(g gin.IRouter) {
	g.GET("/notifications", r.auth.Required(), r.list)
	g.POST("/notifications/:id/_viewed", r.auth.Required(), r.markViewed)
}

func NewNotificationsRoutes(authService auth.A, db database.DB) *NotificationsRoutes {
	return &NotificationsRoutes{
		auth: authService,
		db:   db,
	}
}
