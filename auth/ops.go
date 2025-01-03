package auth

import (
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/logger"
	"net/http"
	"time"
)

const (
	jwtCookieName = "auth-proxy-jwt"
	jwtHeaderKey  = "Authorization"
	jwtQueryParam = "jwt"

	xsrfCookieName = "XSRF-TOKEN"
	xsrfHeaderKey  = "X-XSRF-TOKEN"
	disableXSRF    = false
	cookieSameSite = http.SameSiteNoneMode
	tokenDuration  = time.Minute * 15
	cookieDuration = time.Hour * 24 * 31
)

// Opts holds constructor params
type Opts struct {
	// Configuration for the overall application. Provides many options that control the system.
	Config config.C

	// The service id for the service using this authentication
	ServiceId config.ServiceId

	// UsesQueryParam defines if the auth will accept tokens form the jwt query param. Needed
	// for authorized link-in scenarios for services
	UsesQueryParam bool

	// UsesAuthorizationHeader defines if the auth will accept tokens in the Authorization header. This is needed
	// if the service takes calls from other services or CLI tools.
	UsesAuthorizationHeader bool

	// UsesCookies defines if the auth will accept cookies. This is needed for services that interact with
	// a frontend in the browser.
	UsesCookies bool

	AudSecrets    bool // uses different secret for differed auds. important: adds pre-parsing of unverified token
	SendJWTHeader bool // if enabled send JWT as a header instead of cookie

	Logger       logger.L // logger interface, default is no logging at all
	RefreshCache RefreshCache
	Validator    Validator
}
