package iface

import schemaapi "github.com/rmorlok/authproxy/internal/schema/api"

type InitiateConnectionRequest = schemaapi.InitiateConnectionRequest

type ConnectionSetupResponseType = schemaapi.ConnectionSetupResponseType

const (
	ConnectionSetupResponseTypeRedirect  = schemaapi.ConnectionSetupResponseTypeRedirect
	ConnectionSetupResponseTypeForm      = schemaapi.ConnectionSetupResponseTypeForm
	ConnectionSetupResponseTypeComplete  = schemaapi.ConnectionSetupResponseTypeComplete
	ConnectionSetupResponseTypeVerifying = schemaapi.ConnectionSetupResponseTypeVerifying
	ConnectionSetupResponseTypeError     = schemaapi.ConnectionSetupResponseTypeError
)

type ConnectionSetupResponse = schemaapi.ConnectionSetupResponse
type ConnectionSetupRedirect = schemaapi.ConnectionSetupRedirect
type ConnectionSetupForm = schemaapi.ConnectionSetupForm
type ConnectionSetupComplete = schemaapi.ConnectionSetupComplete
type ConnectionSetupVerifying = schemaapi.ConnectionSetupVerifying
type ConnectionSetupError = schemaapi.ConnectionSetupError
type SubmitConnectionRequest = schemaapi.SubmitConnectionRequest
