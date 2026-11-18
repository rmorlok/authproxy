package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rmorlok/authproxy/internal/apgin"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/httperr"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/stretchr/testify/require"
)

// TestConnectorValidationHTTPError exercises real connector binding and the
// route error writer without external services. Canceled validation is not an
// author's invalid definition; ordinary syntax and limit failures still are.
func TestConnectorValidationHTTPError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []meta.ValidationMode{meta.ValidationModeCreate, meta.ValidationModeUpdate} {
		for name, test := range map[string]struct {
			cause  error
			source string
			status int
		}{
			"canceled":     {cause: context.Canceled, source: "privateSource = ;", status: httperr.StatusClientClosedRequest},
			"deadline":     {cause: context.DeadlineExceeded, source: "privateSource = ;", status: http.StatusInternalServerError},
			"syntax":       {source: "privateSource = ;", status: http.StatusBadRequest},
			"source limit": {source: strings.Repeat(" ", apjs.ConnectorMigrationLimits().MaxSourceBytes+1), status: http.StatusBadRequest},
		} {
			t.Run(string(mode)+"/"+name, func(t *testing.T) {
				// Both create and update bodies carry a migration hook through real schema validation.
				definition := cschema.ConnectorDefinition{Migrations: &cschema.Migrations{
					Up: &cschema.MigrationHook{Javascript: test.source},
				}}
				var body []byte
				var err error
				if mode == meta.ValidationModeCreate {
					body, err = json.Marshal(connectorCreateRequest("root", definition))
				} else {
					body, err = json.Marshal(connectorDefinitionPatch(&definition))
				}
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if test.cause == context.Canceled {
					cancel()
				} else if test.cause == context.DeadlineExceeded {
					var stop context.CancelFunc
					ctx, stop = context.WithDeadline(ctx, time.Time{})
					defer stop()
				}
				response := httptest.NewRecorder()
				gctx, _ := gin.CreateTestContext(response)
				gctx.Request = httptest.NewRequest(http.MethodPut, "/connectors/example", bytes.NewReader(body)).WithContext(ctx)
				if mode == meta.ValidationModeCreate {
					var resource cschema.Connector
					err = apgin.BindResourceJSON(gctx, &resource, mode)
				} else {
					var patch cschema.ConnectorPatch
					err = apgin.BindResourceJSON(gctx, &patch, mode)
				}
				require.Error(t, err)
				apgin.WriteError(gctx, nil, httperr.RequestValidationErr(err, httperr.WithPublicErr(err)))

				// Cancellation keeps its error identity and never presents a migration field error.
				require.Equal(t, test.status, response.Code)
				require.NotContains(t, response.Body.String(), "privateSource")
				if test.cause != nil {
					require.ErrorIs(t, err, test.cause)
					require.NotContains(t, response.Body.String(), "migrations.up.javascript")
					require.NotContains(t, response.Body.String(), "invalid migration javascript")
				} else {
					require.Contains(t, response.Body.String(), "migrations.up.javascript")
					if name == "source limit" {
						require.Contains(t, response.Body.String(), "combined source exceeds 262144 bytes")
					}
				}
			})
		}
	}
}
