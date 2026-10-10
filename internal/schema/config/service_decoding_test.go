package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestServicePublicYAMLDecoding preserves service-specific fields alongside the
// separately decoded HTTP fields and TLS implementation.
func TestServicePublicYAMLDecoding(t *testing.T) {
	const input = `port: 8080
healthCheckPort: 9080
domain: public.example.com
tls:
  autoGenPath: /tmp/public-cert
sessionTimeout: 2h
xsrfRequestQueueDepth: 17
enableMarketplaceApis: false
enableProxy: true
static:
  mountAt: /marketplace
  serveFrom: /srv/public
cookie:
  domain: example.com
  sameSite: lax
`
	var service ServicePublic
	require.NoError(t, yaml.Unmarshal([]byte(input), &service))
	require.EqualValues(t, 8080, service.Port())
	require.EqualValues(t, 9080, service.HealthCheckPort())
	require.Equal(t, "public.example.com", service.DomainVal)
	require.Equal(t, &TlsConfigSelfSignedAutogen{AutoGenPath: "/tmp/public-cert"}, service.TlsVal)
	require.Equal(t, 2*time.Hour, service.SessionTimeout())
	require.Equal(t, 17, service.XsrfRequestQueueDepth())
	require.False(t, service.EnableMarketplaceApis())
	require.True(t, service.EnableProxy())
	require.Equal(t, &ServicePublicStaticContentConfig{MountAtPath: "/marketplace", ServeFromPath: "/srv/public"}, service.StaticVal)
	require.Equal(t, "example.com", *service.CookieVal.DomainVal)
	require.Equal(t, "lax", *service.CookieVal.SameSiteVal)
	checkServiceYAMLReceiver[ServicePublic](t, input, []string{
		"unknown: true", "servicehttp: {}", "ui: {}", "cookie: {unknown: true}",
		"static: {unknown: true}", "sessionTimeout: invalid", "enableProxy: invalid",
		"port: invalid", "tls: {autoGenPath: /tmp/new-cert, unknown: true}", "tls: null",
	})
}

// TestServiceAdminApiYAMLDecoding protects UI and session settings as well as
// shared HTTP configuration when decoding through the method-free type.
func TestServiceAdminApiYAMLDecoding(t *testing.T) {
	const input = `port: 8082
domain: admin.example.com
tls:
  autoGenPath: /tmp/admin-cert
ui:
  enabled: true
  baseUrl: https://admin.example.com
  initiateSessionUrl: https://app.example.com/login
sessionTimeout: 30m
xsrfRequestQueueDepth: 23
static:
  mountAt: /admin
  serveFrom: /srv/admin
cookie:
  domain: example.com
  sameSite: strict
`
	var service ServiceAdminApi
	require.NoError(t, yaml.Unmarshal([]byte(input), &service))
	require.EqualValues(t, 8082, service.Port())
	require.Equal(t, "admin.example.com", service.DomainVal)
	require.Equal(t, &TlsConfigSelfSignedAutogen{AutoGenPath: "/tmp/admin-cert"}, service.TlsVal)
	require.True(t, service.Ui.Enabled)
	require.Equal(t, "https://admin.example.com", service.UiBaseUrl())
	require.Equal(t, "https://app.example.com/login?returnToUrl=https%3A%2F%2Fadmin.example.com", service.Ui.GetInitiateSessionUrl("https://admin.example.com"))
	require.Equal(t, 30*time.Minute, service.SessionTimeout())
	require.Equal(t, 23, service.XsrfRequestQueueDepth())
	require.Equal(t, &ServicePublicStaticContentConfig{MountAtPath: "/admin", ServeFromPath: "/srv/admin"}, service.StaticVal)
	require.Equal(t, "example.com", *service.CookieVal.DomainVal)
	require.Equal(t, "strict", *service.CookieVal.SameSiteVal)
	checkServiceYAMLReceiver[ServiceAdminApi](t, input, []string{
		"unknown: true", "servicehttp: {}", "enableProxy: true", "ui: {unknown: true}",
		"cookie: {unknown: true}", "static: {unknown: true}", "sessionTimeout: invalid",
		"port: invalid", "tls: {autoGenPath: /tmp/new-cert, unknown: true}", "tls: null",
	})
}

// checkServiceYAMLReceiver verifies failed decoding cannot change a populated
// receiver and successful decoding clears fields omitted from the new mapping.
func checkServiceYAMLReceiver[T any](t *testing.T, populated string, invalid []string) {
	t.Helper()
	for _, input := range invalid {
		t.Run(input, func(t *testing.T) {
			var got, expected T
			require.NoError(t, yaml.Unmarshal([]byte(populated), &got))
			require.NoError(t, yaml.Unmarshal([]byte(populated), &expected))
			require.Error(t, yaml.Unmarshal([]byte(input), &got))
			require.Equal(t, expected, got)
		})
	}
	t.Run("omitted fields reset", func(t *testing.T) {
		var got, empty T
		require.NoError(t, yaml.Unmarshal([]byte(populated), &got))
		require.NoError(t, yaml.Unmarshal([]byte("{}"), &got))
		require.Equal(t, empty, got)
	})
}
