package oauth2

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	connIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/request_log"
)

//go:generate mockgen -source=./interface.go -destination=./mock/oauth2.go -package=mock
type Factory interface {
	NewOAuth2(connection database.Connection, connector connIface.ConnectorVersion) OAuth2Connection
	GetOAuth2State(ctx context.Context, actor database.Actor, stateId uuid.UUID) (OAuth2Connection, error)
}

type OAuth2Connection interface {
	RecordCancelSessionAfterAuth(ctx context.Context, shouldCancel bool) error
	CancelSessionAfterAuth() bool
	GenerateAuthUrl(ctx context.Context, actor database.Actor) (string, error)
	SetStateAndGeneratePublicUrl(
		ctx context.Context,
		actor database.Actor,
		returnToUrl string,
	) (string, error)
	CallbackFrom3rdParty(ctx context.Context, query url.Values) (string, error)
	ProxyRequest(ctx context.Context, reqType request_log.RequestType, req *connIface.ProxyRequest) (*connIface.ProxyResponse, error)
	ProxyRequestRaw(ctx context.Context, reqType request_log.RequestType, req *connIface.ProxyRequest, w http.ResponseWriter) error
	SupportsRevokeTokens() bool
	RevokeTokens(ctx context.Context) error
}
