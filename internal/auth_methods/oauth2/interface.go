package oauth2

import (
	"context"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/config/common"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/request_log"
)

type IActorData interface {
	GetId() uuid.UUID
	GetExternalId() string
	GetPermissions() []common.Permission
	IsAdmin() bool
	IsSuperAdmin() bool
	GetEmail() string
}

//go:generate mockgen -source=./interface.go -destination=./mock/oauth2.go -package=mock
type Factory interface {
	NewOAuth2(connection coreIface.Connection) OAuth2Connection
	GetOAuth2State(ctx context.Context, actor IActorData, stateId uuid.UUID) (OAuth2Connection, error)
}

type OAuth2Connection interface {
	RecordCancelSessionAfterAuth(ctx context.Context, shouldCancel bool) error
	CancelSessionAfterAuth() bool
	GenerateAuthUrl(ctx context.Context, actor IActorData) (string, error)
	SetStateAndGeneratePublicUrl(
		ctx context.Context,
		actor IActorData,
		returnToUrl string,
	) (string, error)
	CallbackFrom3rdParty(ctx context.Context, query url.Values) (string, error)
	ProxyRequest(ctx context.Context, reqType request_log.RequestType, req *coreIface.ProxyRequest) (*coreIface.ProxyResponse, error)
	ProxyRequestRaw(ctx context.Context, reqType request_log.RequestType, req *coreIface.ProxyRequest, w http.ResponseWriter) error
	SupportsRevokeTokens() bool
	RevokeTokens(ctx context.Context) error
}
