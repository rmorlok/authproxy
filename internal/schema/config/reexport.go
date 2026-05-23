package config

import (
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/connectors"
)

// Re-export types from the common sub-package
type (
	HumanDuration           = common.HumanDuration
	HumanByteSize           = common.HumanByteSize
	Image                   = common.Image
	ImageBase64             = common.ImageBase64
	ImagePublicUrl          = common.ImagePublicUrl
	IntegerValue            = common.IntegerValue
	IntegerValueDirect      = common.IntegerValueDirect
	IntegerValueEnvVar      = common.IntegerValueEnvVar
	StringValue             = common.StringValue
	StringValueBase64       = common.StringValueBase64
	StringValueDirect       = common.StringValueDirect
	StringValueEnvVar       = common.StringValueEnvVar
	StringValueEnvVarBase64 = common.StringValueEnvVarBase64
	StringValueFile         = common.StringValueFile
)

// Re-export functions from the common sub-package
var (
	KindToString                 = common.KindToString
	MarshalToYamlString          = common.MarshalToYamlString
	MustMarshalToYamlString      = common.MustMarshalToYamlString
	NewStringValueDirect         = common.NewStringValueDirect
	NewStringValueDirectInline   = common.NewStringValueDirectInline
	ValidateNamespacePath        = aschema.ValidateNamespacePath
	SplitNamespacePathToPrefixes = aschema.SplitNamespacePathToPrefixes
	NamespacePathFromRoot        = aschema.NamespacePathFromRoot
)

// Re-export constants from the connectors sub-package
var (
	RootNamespace = aschema.RootNamespace
)

// Re-export types from the connectors sub-package
type (
	Auth                    = connectors.Auth
	AuthType                = connectors.AuthType
	AuthApiKey              = connectors.AuthApiKey
	ApiKeyPlacement         = connectors.ApiKeyPlacement
	AuthOAuth2              = connectors.AuthOAuth2
	AuthNoAuth              = connectors.AuthNoAuth
	AuthOauth2Authorization = connectors.AuthOauth2Authorization
	AuthOauth2PKCE          = connectors.AuthOauth2PKCE
	AuthOauth2Token         = connectors.AuthOauth2Token
	Connector               = connectors.Connector
	Connectors              = connectors.Connectors
	PKCEMethod              = connectors.PKCEMethod
	Scope                   = connectors.Scope
	TokenEndpointAuthMethod = connectors.TokenEndpointAuthMethod
)

// Re-export constants from the connectors sub-package
const (
	AuthTypeOAuth2 = connectors.AuthTypeOAuth2
	AuthTypeAPIKey = connectors.AuthTypeAPIKey

	ApiKeyPlacementBearer = connectors.ApiKeyPlacementBearer
	ApiKeyPlacementHeader = connectors.ApiKeyPlacementHeader
	ApiKeyPlacementQuery  = connectors.ApiKeyPlacementQuery
	ApiKeyPlacementBasic  = connectors.ApiKeyPlacementBasic

	PKCEMethodS256  = connectors.PKCEMethodS256
	PKCEMethodPlain = connectors.PKCEMethodPlain

	TokenEndpointAuthClientSecretPost  = connectors.TokenEndpointAuthClientSecretPost
	TokenEndpointAuthClientSecretBasic = connectors.TokenEndpointAuthClientSecretBasic
	TokenEndpointAuthNone              = connectors.TokenEndpointAuthNone
)
