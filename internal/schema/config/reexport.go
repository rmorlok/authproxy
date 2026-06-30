package config

import (
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	keyschema "github.com/rmorlok/authproxy/internal/schema/resources/key"
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
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
	AwsCredentials          = common.AwsCredentials
	AwsCredentialsImpl      = common.AwsCredentialsImpl
	AwsCredentialsType      = common.AwsCredentialsType
	AwsCredentialsAccessKey = common.AwsCredentialsAccessKey
	AwsCredentialsImplicit  = common.AwsCredentialsImplicit
)

// Re-export functions from the common and namespace sub-packages
var (
	KindToString                 = common.KindToString
	MarshalToYamlString          = common.MarshalToYamlString
	MustMarshalToYamlString      = common.MustMarshalToYamlString
	NewStringValueDirect         = common.NewStringValueDirect
	NewStringValueDirectInline   = common.NewStringValueDirectInline
	ValidateNamespacePath        = nschema.ValidatePath
	SplitNamespacePathToPrefixes = nschema.SplitPathToPrefixes
	NamespacePathFromRoot        = nschema.PathFromRoot
)

// Re-export constants from the namespace sub-package
var (
	RootNamespace = nschema.Root
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
	OAuth2GrantType         = connectors.OAuth2GrantType
	Predicate               = common.Predicate
	Scope                   = connectors.Scope
	ScopeRequired           = connectors.ScopeRequired
	TokenEndpointAuthMethod = connectors.TokenEndpointAuthMethod
)

var (
	NewScopeRequiredBool      = connectors.NewScopeRequiredBool
	NewScopeRequiredPredicate = connectors.NewScopeRequiredPredicate
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

	OAuth2GrantAuthorizationCode = connectors.OAuth2GrantAuthorizationCode
	OAuth2GrantClientCredentials = connectors.OAuth2GrantClientCredentials

	TokenEndpointAuthClientSecretPost  = connectors.TokenEndpointAuthClientSecretPost
	TokenEndpointAuthClientSecretBasic = connectors.TokenEndpointAuthClientSecretBasic
	TokenEndpointAuthNone              = connectors.TokenEndpointAuthNone
)

// Re-export types from the key resource sub-package
type (
	Key                                = keyschema.Key
	KeyType                            = keyschema.KeyType
	KeyShared                          = keyschema.KeyShared
	KeyPublicPrivate                   = keyschema.KeyPublicPrivate
	KeyData                            = keyschema.KeyData
	KeyDataType                        = keyschema.KeyDataType
	KeyDataValue                       = keyschema.KeyDataValue
	KeyDataBase64Val                   = keyschema.KeyDataBase64Val
	KeyDataEnvVar                      = keyschema.KeyDataEnvVar
	KeyDataEnvBase64Var                = keyschema.KeyDataEnvBase64Var
	KeyDataFile                        = keyschema.KeyDataFile
	KeyDataRandomBytes                 = keyschema.KeyDataRandomBytes
	KeyDataAwsSecret                   = keyschema.KeyDataAwsSecret
	KeyDataAwsKMS                      = keyschema.KeyDataAwsKMS
	KeyDataGcpSecret                   = keyschema.KeyDataGcpSecret
	KeyDataGcpKMS                      = keyschema.KeyDataGcpKMS
	KeyDataVault                       = keyschema.KeyDataVault
	KeyDataVaultTransit                = keyschema.KeyDataVaultTransit
	KeyDataMock                        = keyschema.KeyDataMock
	KeyDataMockKMS                     = keyschema.KeyDataMockKMS
	KeyDataRawVal                      = keyschema.KeyDataRawVal
	ProviderType                       = keyschema.ProviderType
	KeyVersionProtectedData            = keyschema.KeyVersionProtectedData
	KeyVersionInfo                     = keyschema.KeyVersionInfo
	DataEncryptionKeyInfo              = keyschema.DataEncryptionKeyInfo
	KeyWrappingKeyInfo                 = keyschema.KeyWrappingKeyInfo
	GeneratedDataEncryptionKey         = keyschema.GeneratedDataEncryptionKey
	KeyDataRequiresDataEncryptionKeys  = keyschema.KeyDataRequiresDataEncryptionKeys
	KeyDataWrapsDataEncryptionKeys     = keyschema.KeyDataWrapsDataEncryptionKeys
	KeyDataGeneratesDataEncryptionKeys = keyschema.KeyDataGeneratesDataEncryptionKeys
)

var (
	DataHash                    = keyschema.DataHash
	NewKeyDataRandomBytes       = keyschema.NewKeyDataRandomBytes
	ResetKeyDataMockRegistry    = keyschema.ResetKeyDataMockRegistry
	NewKeyDataMock              = keyschema.NewKeyDataMock
	KeyDataMockAddVersion       = keyschema.KeyDataMockAddVersion
	KeyDataMockSetVersions      = keyschema.KeyDataMockSetVersions
	KeyDataMockRemoveVersion    = keyschema.KeyDataMockRemoveVersion
	ResetKeyDataMockKMSRegistry = keyschema.ResetKeyDataMockKMSRegistry
	NewKeyDataMockKMS           = keyschema.NewKeyDataMockKMS
	KeyDataMockKMSAddVersion    = keyschema.KeyDataMockKMSAddVersion
	KeyDataMockKMSWrap          = keyschema.KeyDataMockKMSWrap
)

// Re-export constants from the common and key resource sub-packages
const (
	AwsCredentialsTypeAccessKey = common.AwsCredentialsTypeAccessKey
	AwsCredentialsTypeImplicit  = common.AwsCredentialsTypeImplicit

	ProviderTypeValue                 = keyschema.ProviderTypeValue
	ProviderTypeBase64                = keyschema.ProviderTypeBase64
	ProviderTypeEnvVar                = keyschema.ProviderTypeEnvVar
	ProviderTypeEnvVarBase64          = keyschema.ProviderTypeEnvVarBase64
	ProviderTypeFile                  = keyschema.ProviderTypeFile
	ProviderTypeRandom                = keyschema.ProviderTypeRandom
	ProviderTypeAwsSecretsManager     = keyschema.ProviderTypeAwsSecretsManager
	ProviderTypeAwsKMS                = keyschema.ProviderTypeAwsKMS
	ProviderTypeGcp                   = keyschema.ProviderTypeGcp
	ProviderTypeGcpKMS                = keyschema.ProviderTypeGcpKMS
	ProviderTypeHashicorpVault        = keyschema.ProviderTypeHashicorpVault
	ProviderTypeHashicorpVaultTransit = keyschema.ProviderTypeHashicorpVaultTransit
	ProviderTypeRaw                   = keyschema.ProviderTypeRaw
	ProviderTypeMock                  = keyschema.ProviderTypeMock
	ProviderTypeMockKMS               = keyschema.ProviderTypeMockKMS

	DataEncryptionKeySize                      = keyschema.DataEncryptionKeySize
	KeyVersionProtectedDataTypeAuthProxyAESGCM = keyschema.KeyVersionProtectedDataTypeAuthProxyAESGCM
)
