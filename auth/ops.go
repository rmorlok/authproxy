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
	Config      *config.Root
	ApiHost     *config.ApiHost
	UsesCookies bool

	SecretReader SecretReader
	ClaimsUpd    ClaimsUpdater

	AudienceReader Audience // allowed aud values
	AudSecrets     bool     // uses different secret for differed auds. important: adds pre-parsing of unverified token
	SendJWTHeader  bool     // if enabled send JWT as a header instead of cookie

	Logger       logger.L // logger interface, default is no logging at all
	RefreshCache RefreshCache
	Validator    Validator
}
