package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestHTTPServiceYAMLDecoding checks all canonical HTTP fields, including the
// inline health port, while TLS continues to use its concrete-type dispatch.
func TestHTTPServiceYAMLDecoding(t *testing.T) {
	const common = `port: 8443
healthCheckPort: 9080
baseUrl: https://external.example.com/base/
domain: service.example.com
https: true
cors:
  allowedOrigins: [https://app.example.com]
  allowedMethods: [GET]
  allowedHeaders: [Authorization]
  exposedHeaders: [X-Request-Id]
  maxAge: 10m
  allowCredentials: true
`
	cases := []struct {
		name string
		tls  string
		want TlsConfig
	}{
		{name: "omitted"},
		{name: "self signed", tls: "tls: {autoGenPath: /tmp/service-cert}\n", want: &TlsConfigSelfSignedAutogen{AutoGenPath: "/tmp/service-cert"}},
		{name: "lets encrypt", tls: "tls: {acceptTos: true, email: admin@example.com, hostWhitelist: [service.example.com], cacheDir: /tmp/cache}\n", want: &TlsConfigLetsEncrypt{AcceptTos: true, Email: "admin@example.com", HostWhitelist: []string{"service.example.com"}, CacheDir: "/tmp/cache"}},
		{name: "certificate values", tls: "tls: {cert: null, key: null}\n", want: &TlsConfigVals{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var service ServiceApi
			require.NoError(t, yaml.Unmarshal([]byte(common+tc.tls), &service))
			require.EqualValues(t, 8443, service.Port())
			require.EqualValues(t, 9080, service.HealthCheckPort())
			require.Equal(t, "https://external.example.com/base", service.GetBaseUrl())
			require.Equal(t, "service.example.com", service.DomainVal)
			require.True(t, service.IsHttpsVal)
			require.Equal(t, tc.want, service.TlsVal)
			require.Equal(t, []string{"https://app.example.com"}, service.CorsVal.AllowedOrigins)
			require.Equal(t, []string{"GET"}, service.CorsVal.AllowedMethods)
			require.Equal(t, []string{"Authorization"}, service.CorsVal.AllowedHeaders)
			require.Equal(t, []string{"X-Request-Id"}, service.CorsVal.ExposedHeaders)
			require.Equal(t, 10*time.Minute, service.CorsVal.MaxAge.Duration)
			require.True(t, *service.CorsVal.AllowCredentials)
			require.NoError(t, yaml.Unmarshal([]byte("{}"), &service))
			require.Equal(t, ServiceApi{}, service)
		})
	}
}

// TestHTTPServiceYAMLDecodeErrorsLeaveReceiverUnchanged protects strict field
// checks and atomic assignment even when HTTP or TLS decoding fails late.
func TestHTTPServiceYAMLDecodeErrorsLeaveReceiverUnchanged(t *testing.T) {
	const initial = "port: 8081\ndomain: original.example.com\ntls: {autoGenPath: /tmp/original-cert}\n"
	for _, input := range []string{
		"unknown: true", "serviceCommon: {}", "sessionTimeout: 1h",
		"port: invalid", "healthCheckPort: invalid", "cors: {unknown: true}",
		"tls: {autoGenPath: /tmp/new-cert, unknown: true}", "tls: {}", "tls: null",
	} {
		t.Run(input, func(t *testing.T) {
			var got, expected ServiceApi
			require.NoError(t, yaml.Unmarshal([]byte(initial), &got))
			require.NoError(t, yaml.Unmarshal([]byte(initial), &expected))
			require.Error(t, yaml.Unmarshal([]byte(input), &got))
			require.Equal(t, expected, got)
		})
	}
}
