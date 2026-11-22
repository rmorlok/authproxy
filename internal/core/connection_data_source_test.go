package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	"github.com/rmorlok/authproxy/internal/httperr"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDataSource(t *testing.T) {
	t.Run("transforms response with connector javascript context", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, proxy := newProbeTestConnection(t, ctrl, cschema.ConnectorDefinition{
			Javascript: `
				function workspaceOptions(items) {
					return items.map(function(item) {
						return {
							value: labels.env + ":" + item.id,
							label: cfg.region + "/" + annotations.prefix + "/" + item.summary
						};
					});
				}
			`,
			SetupFlow: &cschema.SetupFlow{
				Configure: &cschema.SetupFlowPhase{
					Steps: []cschema.SetupFlowStep{
						{
							Id:         "workspace",
							JsonSchema: workspaceSchema,
							DataSources: map[string]cschema.DataSourceDef{
								"workspaces": {
									ProxyRequest: &cschema.DataSourceProxyRequest{
										Method:  "GET",
										Url:     "https://api.example.com/{{cfg.region}}/workspaces",
										Headers: map[string]string{"X-Environment": "{{labels.env}}"},
									},
									Transform: "workspaceOptions(data.items)",
								},
							},
						},
					},
				},
			},
		})
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step
		conn.s.encrypt = encrypt.NewFakeEncryptService(false)
		setConnectionConfigFixture(t, conn, map[string]any{"region": "us-east"})
		conn.Labels = map[string]string{"env": "prod"}
		conn.Annotations = map[string]string{"prefix": "team"}
		proxy.resp = &iface.ProxyResponse{
			StatusCode: 200,
			BodyJson: map[string]any{
				"items": []any{
					map[string]any{"id": "primary", "summary": "Primary"},
				},
			},
		}

		options, err := conn.GetDataSource(context.Background(), "workspaces")
		require.NoError(t, err)
		require.Len(t, options, 1)
		assert.Equal(t, "prod:primary", options[0].Value)
		assert.Equal(t, "us-east/team/Primary", options[0].Label)
		require.Len(t, proxy.calls, 1)
		assert.Equal(t, "https://api.example.com/us-east/workspaces", proxy.calls[0].URL)
		assert.Equal(t, common.HeadersValMapFromStrings(map[string]string{"X-Environment": "prod"}), proxy.calls[0].Headers)
	})

	t.Run("returns error when no setup step", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{
						Id:         "workspace",
						JsonSchema: workspaceSchema,
						DataSources: map[string]cschema.DataSourceDef{
							"workspaces": {
								ProxyRequest: &cschema.DataSourceProxyRequest{
									Method: "GET",
									Url:    "https://api.example.com/workspaces",
								},
								Transform: "data",
							},
						},
					},
				},
			},
		})

		_, err := conn.GetDataSource(context.Background(), "workspaces")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no active setup step")
	})

	t.Run("returns error when in preconnect phase", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Preconnect: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "tenant", JsonSchema: tenantSchema},
				},
			},
		})
		step := cschema.MustNewSetupStep("tenant")
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "some_source")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only available during configure steps")
	})

	t.Run("returns error when in auth phase", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{Id: "workspace", JsonSchema: workspaceSchema},
				},
			},
		})
		// A pseudo-step (or any non-configure step) gates data-source access.
		step := cschema.SetupStepVerify
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "some_source")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only available during configure steps")
	})

	t.Run("returns error when no setup flow on connector", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, nil)
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "workspaces")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no setup flow")
	})

	t.Run("returns error when data source not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{
						Id: "workspace",
						JsonSchema: common.RawJSON(`{
							"type": "object",
							"properties": {
								"workspace_id": {"type": "string"}
							}
						}`),
					},
				},
			},
		})
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found in current step")
	})

	t.Run("returns error when data source has no proxy request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		conn, _ := newTestConnectionWithSetupFlow(t, ctrl, &cschema.SetupFlow{
			Configure: &cschema.SetupFlowPhase{
				Steps: []cschema.SetupFlowStep{
					{
						Id: "workspace",
						JsonSchema: common.RawJSON(`{
							"type": "object",
							"properties": {
								"workspace_id": {"type": "string"}
							}
						}`),
						DataSources: map[string]cschema.DataSourceDef{
							"workspaces": {
								Transform: "data",
							},
						},
					},
				},
			},
		})
		step := cschema.MustNewSetupStep("workspace")
		conn.SetupStep = &step

		_, err := conn.GetDataSource(context.Background(), "workspaces")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no proxy_request defined")
	})
}

// dataSourceTestDefinition supplies one configure-step source for bounded
// transform tests; the source's HTTP contract is intentionally ordinary.
func dataSourceTestDefinition(library, transform string) cschema.ConnectorDefinition {
	return cschema.ConnectorDefinition{
		Javascript: library,
		SetupFlow: &cschema.SetupFlow{Configure: &cschema.SetupFlowPhase{Steps: []cschema.SetupFlowStep{{
			Id: "workspace", JsonSchema: workspaceSchema,
			DataSources: map[string]cschema.DataSourceDef{"workspaces": {
				ProxyRequest: &cschema.DataSourceProxyRequest{Method: "GET", Url: "https://api.example.com/workspaces"},
				Transform:    transform,
			}},
		}}}},
	}
}

// boundedDataSourceTestConnection provides cached configuration and a recording
// proxy while leaving the connector definition uncached for contextual loading.
func boundedDataSourceTestConnection(t *testing.T, definition cschema.ConnectorDefinition) (*connection, *stubProxy) {
	t.Helper()
	conn, proxy := newProbeTestConnection(t, gomock.NewController(t), definition)
	step := cschema.MustNewSetupStep("workspace")
	conn.SetupStep = &step
	conn.configLoaded = true
	proxy.resp = &iface.ProxyResponse{StatusCode: http.StatusOK, BodyJson: map[string]any{}}
	return conn, proxy
}

// dataSourceContextProxy replaces only buffered HTTP so tests can observe the
// request context or cancel it when the response arrives.
type dataSourceContextProxy struct {
	iface.Proxy
	request func(context.Context, httpf.RequestType, *iface.ProxyRequest) (*iface.ProxyResponse, error)
}

// ProxyRequest delegates buffered requests to the test's context-aware callback.
func (p *dataSourceContextProxy) ProxyRequest(ctx context.Context, kind httpf.RequestType, request *iface.ProxyRequest) (*iface.ProxyResponse, error) {
	return p.request(ctx, kind, request)
}

// TestGetDataSourceBoundedFailures checks that source admission precedes HTTP
// while input, execution, and output failures remain bounded and private.
func TestGetDataSourceBoundedFailures(t *testing.T) {
	for name, test := range map[string]struct {
		library, transform string
		data, config       map[string]any
		want               error
		beforeHTTP         bool
	}{
		"library syntax":      {library: "const privateSource = ;", transform: "[]", want: jsruntime.ErrInvalidSource, beforeHTTP: true},
		"combined raw source": {library: strings.Repeat(" ", apjs.ConnectorDataSourceLimits().MaxSourceBytes), transform: "[]", want: jsruntime.ErrLimitExceeded, beforeHTTP: true},
		"transform source":    {transform: strings.Repeat(" ", apjs.ConnectorDataSourceLimits().MaxSourceBytes) + "[]", want: jsruntime.ErrLimitExceeded, beforeHTTP: true},
		"library timeout":     {library: "for (;;) {}", transform: "[]", want: jsruntime.ErrLimitExceeded},
		"transform timeout":   {transform: "for (;;) {}", want: jsruntime.ErrLimitExceeded},
		"transform syntax":    {transform: "const privateSource = ;", want: jsruntime.ErrInvalidSource},
		"private throw":       {transform: `throw new Error("privateConnectionValue")`, want: jsruntime.ErrEvaluation},
		"invalid output":      {transform: `({value: "privateOutput"})`, want: jsruntime.ErrInvalidOutput},
		"output bytes":        {transform: `[{value: "x".repeat(1 << 20), label: "Large"}]`, want: jsruntime.ErrLimitExceeded},
		"combined input bytes": {
			transform: "[]", want: jsruntime.ErrLimitExceeded,
			data:   map[string]any{"padding": strings.Repeat("x", apjs.ConnectorDataSourceLimits().MaxInputBytes/2)},
			config: map[string]any{"padding": strings.Repeat("x", apjs.ConnectorDataSourceLimits().MaxInputBytes/2)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			// The source is eligible and HTTP succeeds, isolating the transform boundary.
			conn, proxy := boundedDataSourceTestConnection(t, dataSourceTestDefinition(test.library, test.transform))
			conn.configCache = test.config
			if test.data != nil {
				proxy.resp.BodyJson = test.data
			}
			options, err := conn.GetDataSource(context.Background(), "workspaces")

			// Safe classifications survive HTTP wrappers and no partial options escape.
			require.ErrorIs(t, err, test.want)
			require.NotContains(t, err.Error(), "private")
			require.Nil(t, options)
			if test.beforeHTTP {
				require.Empty(t, proxy.calls)
				if test.want == jsruntime.ErrLimitExceeded {
					require.False(t, conn.connector.jsLoaded)
				}
			} else {
				require.Len(t, proxy.calls, 1)
			}
		})
	}
}

// TestGetDataSourceContext keeps HTTP on the caller's context and prevents
// canceled fetches or evaluations from producing options or unrelated failures.
func TestGetDataSourceContext(t *testing.T) {
	// Already-canceled and missing contexts stop before definition loading or HTTP.
	conn, proxy := boundedDataSourceTestConnection(t, dataSourceTestDefinition("", "[]"))
	options, err := conn.GetDataSource(nil, "workspaces")
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, options)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	options, err = conn.GetDataSource(ctx, "workspaces")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, options)
	require.Nil(t, conn.connector.def)
	require.Empty(t, proxy.calls)

	// The HTTP call receives the exact caller context, not the JavaScript policy deadline.
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn.proxyImpl = &dataSourceContextProxy{Proxy: proxy, request: func(requestCtx context.Context, kind httpf.RequestType, request *iface.ProxyRequest) (*iface.ProxyResponse, error) {
		require.Same(t, ctx, requestCtx)
		require.Equal(t, httpf.RequestTypeProxy, kind)
		require.Equal(t, http.MethodGet, request.Method)
		cancel()
		return &iface.ProxyResponse{StatusCode: http.StatusServiceUnavailable}, nil
	}}
	options, err = conn.GetDataSource(ctx, "workspaces")
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, options)

	// A caller deadline also interrupts transform execution before its longer policy timeout.
	conn, proxy = boundedDataSourceTestConnection(t, dataSourceTestDefinition("", "for (;;) {}"))
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	options, err = conn.GetDataSource(ctx, "workspaces")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, options)
	require.Len(t, proxy.calls, 1)
}

// TestGetDataSourceDefinitionRetry preserves contextual infrastructure errors
// without panicking or poisoning the definition/library caches after a failure.
func TestGetDataSourceDefinitionRetry(t *testing.T) {
	for _, name := range []string{"decrypt", "decode", "cancel"} {
		t.Run(name, func(t *testing.T) {
			// Fail the first uncached load under the actual request context.
			definition := dataSourceTestDefinition("", "[]")
			conn, proxy := boundedDataSourceTestConnection(t, definition)
			encryption := encryptmock.NewMockE(gomock.NewController(t))
			conn.connector.s.encrypt = encryption
			encoded, err := json.Marshal(definition)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			loadFailure := errors.New("definition decryption unavailable")
			encryption.EXPECT().DecryptString(ctx, conn.connector.EncryptedDefinition).DoAndReturn(func(context.Context, encfield.EncryptedField) (string, error) {
				switch name {
				case "decrypt":
					return "", loadFailure
				case "decode":
					return "{", nil
				default:
					cancel()
					return string(encoded), nil
				}
			})
			options, err := conn.GetDataSource(ctx, "workspaces")
			require.Nil(t, options)
			require.Error(t, err)
			switch name {
			case "decrypt":
				require.ErrorIs(t, err, loadFailure)
			case "decode":
				var syntaxError *json.SyntaxError
				require.ErrorAs(t, err, &syntaxError)
			default:
				require.ErrorIs(t, err, context.Canceled)
			}
			require.Nil(t, conn.connector.def)
			require.False(t, conn.connector.jsLoaded)
			require.Empty(t, proxy.calls)

			// A live retry loads once; later setup/proxy helpers reuse that captured definition.
			encryption.EXPECT().DecryptString(context.Background(), conn.connector.EncryptedDefinition).Return(string(encoded), nil)
			options, err = conn.GetDataSource(context.Background(), "workspaces")
			require.NoError(t, err)
			require.Empty(t, options)
			require.Len(t, proxy.calls, 1)
		})
	}
}

// TestGetDataSourceInputIsolation copies upstream and connection input while
// retaining numeric option values in the returned string-based options.
func TestGetDataSourceInputIsolation(t *testing.T) {
	// Mutating every input in JavaScript must affect only this invocation's native copy.
	conn, proxy := boundedDataSourceTestConnection(t, dataSourceTestDefinition("", `
		cfg.region = "changed"; labels.team = "changed"; annotations.note = "changed";
		data.name = "changed"; [{value: 42, label: data.name}];
	`))
	conn.configCache = map[string]any{"region": "original"}
	conn.Labels = map[string]string{"team": "original"}
	conn.Annotations = map[string]string{"note": "original"}
	data := map[string]any{"name": "original"}
	proxy.resp.BodyJson = data
	options, err := conn.GetDataSource(context.Background(), "workspaces")
	require.NoError(t, err)
	require.Equal(t, []apjs.DataSourceOption{{Value: "42", Label: "changed"}}, options)
	require.Equal(t, "original", conn.configCache["region"])
	require.Equal(t, "original", conn.Labels["team"])
	require.Equal(t, "original", conn.Annotations["note"])
	require.Equal(t, "original", data["name"])
}

// TestGetDataSourceAdmissionAndHTTPFailures preserves setup eligibility and
// provider-failure responses independently of the new JavaScript policy.
func TestGetDataSourceAdmissionAndHTTPFailures(t *testing.T) {
	// A configured but ineligible step cannot dispatch its data source request.
	definition := dataSourceTestDefinition("", "[]")
	definition.SetupFlow.Configure.Steps[0].If = &common.Predicate{Javascript: "false"}
	conn, proxy := boundedDataSourceTestConnection(t, definition)
	options, err := conn.GetDataSource(context.Background(), "workspaces")
	require.Nil(t, options)
	require.Equal(t, http.StatusBadRequest, httperr.FromError(err).Status)
	require.ErrorContains(t, err, "not eligible")
	require.Empty(t, proxy.calls)

	for _, name := range []string{"network", "status", "non-JSON"} {
		t.Run(name, func(t *testing.T) {
			// The same buffered proxy failures remain bad-gateway responses.
			conn, proxy := boundedDataSourceTestConnection(t, dataSourceTestDefinition("", "[]"))
			upstreamFailure := errors.New("upstream unavailable")
			switch name {
			case "network":
				proxy.err = upstreamFailure
			case "status":
				proxy.resp.StatusCode = http.StatusServiceUnavailable
			default:
				proxy.resp.BodyJson = nil
			}
			options, err := conn.GetDataSource(context.Background(), "workspaces")
			require.Nil(t, options)
			require.Equal(t, http.StatusBadGateway, httperr.FromError(err).Status)
			if name == "network" {
				require.ErrorIs(t, err, upstreamFailure)
			}
		})
	}
}
