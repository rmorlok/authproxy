package oauth2

import (
	"net/url"
	"strings"
)

const defaultOAuthReturnPath = "/connections"

func (o *oAuth2Connection) safeReturnToUrl(raw string) string {
	fallback := o.defaultReturnToUrl()

	returnURL, err := url.Parse(raw)
	if err != nil || returnURL.Scheme == "" || returnURL.Host == "" {
		return fallback
	}

	if returnURL.User != nil || !isAllowedReturnScheme(returnURL.Scheme) {
		return fallback
	}

	publicURL, err := url.Parse(o.cfg.GetRoot().Public.GetBaseUrl())
	if err != nil {
		return fallback
	}

	if !sameOrigin(publicURL, returnURL) {
		return fallback
	}

	return returnURL.String()
}

func (o *oAuth2Connection) defaultReturnToUrl() string {
	raw := o.cfg.GetRoot().Public.GetBaseUrl()
	publicURL, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	publicURL.Path = defaultOAuthReturnPath
	publicURL.RawQuery = ""
	publicURL.Fragment = ""
	return publicURL.String()
}

func isAllowedReturnScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
