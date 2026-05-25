package oauth2

import (
	"context"
	"net/url"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/auth_methods"
	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
)

type IActorData interface {
	GetId() apid.ID
	GetExternalId() string
	GetLabels() map[string]string
	GetPermissions() []aschema.Permission
	GetNamespace() string
}

//go:generate mockgen -source=./interface.go -destination=./mock/oauth2.go -package=mock
type Factory interface {
	NewOAuth2(connection coreIface.Connection) OAuth2Connection
	NewAuthenticator(connection coreIface.Connection) auth_methods.Authenticator
	GetOAuth2State(ctx context.Context, actor IActorData, stateId apid.ID) (OAuth2Connection, error)
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
}
