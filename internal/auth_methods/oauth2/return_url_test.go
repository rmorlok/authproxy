package oauth2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeReturnToUrl_AllowsPublicOrigin(t *testing.T) {
	o := &oAuth2Connection{cfg: configWithPublicBaseUrl(t, "proxy.example.com")}

	got := o.safeReturnToUrl("http://proxy.example.com/connections?tab=integrations")

	assert.Equal(t, "http://proxy.example.com/connections?tab=integrations", got)
}

func TestSafeReturnToUrl_RejectsExternalOrigin(t *testing.T) {
	o := &oAuth2Connection{cfg: configWithPublicBaseUrl(t, "proxy.example.com")}

	got := o.safeReturnToUrl("https://evil.example/phish")

	assert.Equal(t, "http://proxy.example.com/connections", got)
}

func TestSafeReturnToUrl_RejectsAmbiguousOrUnsafeUrls(t *testing.T) {
	o := &oAuth2Connection{cfg: configWithPublicBaseUrl(t, "proxy.example.com")}

	for _, raw := range []string{
		"/connections",
		"javascript:alert(1)",
		"http://proxy.example.com@evil.example/connections",
		"http://proxy.example.com/\x7f",
	} {
		assert.Equalf(t, "http://proxy.example.com/connections", o.safeReturnToUrl(raw), "raw=%q", raw)
	}
}
