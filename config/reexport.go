package config

import (
	"github.com/rmorlok/authproxy/config/common"
	"github.com/rmorlok/authproxy/config/connectors"
)

// Re-export types from the common sub-package
type (
	HumanDuration = common.HumanDuration
	Image         = common.Image
	ImageBase64   = common.ImageBase64
	ImagePublicUrl = common.ImagePublicUrl
	StringValue   = common.StringValue
	StringValueBase64 = common.StringValueBase64
	StringValueDirect = common.StringValueDirect
	StringValueEnvVar = common.StringValueEnvVar
	StringValueEnvVarBase64 = common.StringValueEnvVarBase64
	StringValueFile = common.StringValueFile
)

// Re-export functions from the common sub-package
var (
	KindToString = common.KindToString
	MarshalToYamlString = common.MarshalToYamlString
	MustMarshalToYamlString = common.MustMarshalToYamlString
	UnmarshallYamlImage = common.UnmarshallYamlImage
	UnmarshallYamlImageString = common.UnmarshallYamlImageString
	ImageUnmarshalYAML = common.ImageUnmarshalYAML
	UnmarshallYamlStringValue = common.UnmarshallYamlStringValue
	UnmarshallYamlStringValueString = common.UnmarshallYamlStringValueString
	StringValueUnmarshalYAML = common.StringValueUnmarshalYAML
)

// Re-export types from the connectors sub-package
type (
	Auth = connectors.Auth
	AuthType = connectors.AuthType
	AuthApiKey = connectors.AuthApiKey
	AuthOAuth2 = connectors.AuthOAuth2
	AuthOauth2Authorization = connectors.AuthOauth2Authorization
	AuthOauth2Token = connectors.AuthOauth2Token
	Connector = connectors.Connector
	Scope = connectors.Scope
)

// Re-export constants from the connectors sub-package
const (
	AuthTypeOAuth2 = connectors.AuthTypeOAuth2
	AuthTypeAPIKey = connectors.AuthTypeAPIKey
)

// Re-export functions from the connectors sub-package
var (
	UnmarshallYamlAuth = connectors.UnmarshallYamlAuth
	UnmarshallYamlAuthString = connectors.UnmarshallYamlAuthString
	UnmarshallYamlScope = connectors.UnmarshallYamlScope
	UnmarshallYamlScopeString = connectors.UnmarshallYamlScopeString
)