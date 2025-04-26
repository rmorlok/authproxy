package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/auth"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/proxy"
)

func (r *ConnectionsRoutes) proxy(gctx *gin.Context) {
	ctx := gctx.Request.Context()
	ra := auth.GetAuthFromGinContext(gctx)
	if !ra.IsAuthenticated() {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusUnauthorized().
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	id := gctx.Param("id")
	connectionUuid, err := uuid.Parse(id)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid connection id").
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if connectionUuid == uuid.Nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("connection id is required").
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	connection, err := r.db.GetConnection(ctx, connectionUuid)
	if err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if connection == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusNotFound().
			WithResponseMsg("connection not found").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	// TODO: add security checking for ownership

	var connector *config.Connector
	for _, c := range r.cfg.GetRoot().Connectors {
		if c.Id == connection.ConnectorId {
			connector = &c
		}
	}

	if connector == nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusInternalServerError().
			WithResponseMsg("could not find connector for connection").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	var proxyRequest proxy.ProxyRequest
	if err := gctx.ShouldBindJSON(&proxyRequest); err != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("invalid proxy request payload").
			WithInternalErr(err).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	validateErr := proxyRequest.Validate()
	if validateErr != nil {
		api_common.NewHttpStatusErrorBuilder().
			WithInternalErr(validateErr).
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}

	if _, ok := connector.Auth.(*config.AuthOAuth2); ok {
		o2 := r.oauthf.NewOAuth2(*connection, *connector)
		resp, err := o2.ProxyRequest(ctx, &proxyRequest)
		if err != nil {
			api_common.NewHttpStatusErrorBuilder().
				WithInternalErr(err).
				BuildStatusError().
				WriteGinResponse(r.cfg, gctx)
			return
		}

		gctx.PureJSON(200, resp)
	} else {
		api_common.NewHttpStatusErrorBuilder().
			WithStatusBadRequest().
			WithResponseMsg("connector type does not support proxying").
			BuildStatusError().
			WriteGinResponse(r.cfg, gctx)
		return
	}
}
