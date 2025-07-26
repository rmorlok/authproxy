package oauth2

import (
	"context"
	"github.com/google/uuid"
	connIface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/proxy"
	"net/http"
	"net/url"
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
	ProxyRequest(ctx context.Context, req *proxy.ProxyRequest) (*proxy.ProxyResponse, error)
	ProxyRequestRaw(ctx context.Context, req *proxy.ProxyRequest, w http.ResponseWriter) error
	SupportsRevokeRefreshToken() bool
	RevokeRefreshToken(ctx context.Context) error
}
