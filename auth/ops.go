package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/logger"
	"github.com/rmorlok/authproxy/redis"
)

const (
	JwtHeaderKey  = "Authorization"
	JwtQueryParam = "jwt"
)

// Opts holds constructor params
type Opts struct {
	// Configuration for the overall application. Provides many options that control the system.
	Config config.C

	// The service using this authentication
	Service config.Service

	AudSecrets    bool // uses different secret for differed auds. important: adds pre-parsing of unverified token
	SendJWTHeader bool // if enabled send JWT as a header instead of cookie

	Logger    logger.L // logger interface, default is no logging at all
	Validator Validator
	Db        database.DB
	Redis     redis.R
}
