package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apredis"
	apredis2 "github.com/rmorlok/authproxy/internal/apredis"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/jwt"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// HandlerFunc mirrors gin.HandlerFunc but injects the auth object into the handler so you can do things like
// manage session.
type HandlerFunc func(c *gin.Context, a A)

type route struct {
	method     string
	path       string
	handler    HandlerFunc
	validators []ActorValidator
}

type TestGinServerBuilder struct {
	testName                          string
	pingCounter                       int
	service                           config.ServiceId
	cfg                               config.C
	db                                database.DB
	r                                 apredis.Client
	ginEngine                         *gin.Engine
	openRoutes                        []route
	optionalAuthRoutes                []route
	optionalXsrfNotRequiredAuthRoutes []route
	requiredAuthRoutes                []route
	adminAuthRoutes                   []route
	defaultValidators                 []ActorValidator
}

func NewTestGinServerBuilder(testName string) *TestGinServerBuilder {
	return &TestGinServerBuilder{testName: testName}
}

func (b *TestGinServerBuilder) WithDefaultValidator(v ActorValidator) *TestGinServerBuilder {
	b.defaultValidators = append(b.defaultValidators, v)
	return b
}

func (b *TestGinServerBuilder) WithConfig(cfg config.C) *TestGinServerBuilder {
	b.cfg = cfg
	return b
}

func (b *TestGinServerBuilder) WithDb(db database.DB) *TestGinServerBuilder {
	b.db = db
	return b
}

func (b *TestGinServerBuilder) WitRedis(r apredis.Client) *TestGinServerBuilder {
	b.r = r
	return b
}

func (b *TestGinServerBuilder) WithOpenRoute(method, path string, handler HandlerFunc) *TestGinServerBuilder {
	b.openRoutes = append(b.openRoutes, route{path: path, handler: handler, method: method})
	return b
}

func (b *TestGinServerBuilder) WithGetPingOpenRoute(path string) *TestGinServerBuilder {
	return b.WithOpenRoute(http.MethodGet, path, func(c *gin.Context, a A) {
		b.pingCounter++
		c.PureJSON(200, gin.H{"ok": true})
	})
}

func (b *TestGinServerBuilder) WithOptionalAuthRoute(method, path string, handler HandlerFunc, validators ...ActorValidator) *TestGinServerBuilder {
	b.optionalAuthRoutes = append(b.optionalAuthRoutes, route{path: path, handler: handler, method: method, validators: validators})
	return b
}

func (b *TestGinServerBuilder) WithGetPingOptionalAuthRoute(path string, validators ...ActorValidator) *TestGinServerBuilder {
	return b.WithOptionalAuthRoute(http.MethodGet, path, func(c *gin.Context, a A) {
		b.pingCounter++
		c.PureJSON(200, gin.H{"ok": true})
	}, validators...)
}

func (b *TestGinServerBuilder) WithOptionalXsrfNotRequiredAuthRoute(method, path string, handler HandlerFunc, validators ...ActorValidator) *TestGinServerBuilder {
	b.optionalXsrfNotRequiredAuthRoutes = append(b.optionalXsrfNotRequiredAuthRoutes, route{path: path, handler: handler, method: method, validators: validators})
	return b
}

func (b *TestGinServerBuilder) WithGetPingOptionalXsrfNotRequiredAuthRoute(path string, validators ...ActorValidator) *TestGinServerBuilder {
	return b.WithOptionalXsrfNotRequiredAuthRoute(http.MethodGet, path, func(c *gin.Context, a A) {
		b.pingCounter++
		c.PureJSON(200, gin.H{"ok": true})
	}, validators...)
}

func (b *TestGinServerBuilder) WithRequiredAuthRoute(method, path string, handler HandlerFunc, validators ...ActorValidator) *TestGinServerBuilder {
	b.requiredAuthRoutes = append(b.requiredAuthRoutes, route{path: path, handler: handler, method: method, validators: validators})
	return b
}

func (b *TestGinServerBuilder) WithGetPingRequiredAuthRoute(path string, validators ...ActorValidator) *TestGinServerBuilder {
	return b.WithRequiredAuthRoute(http.MethodGet, path, func(c *gin.Context, a A) {
		b.pingCounter++
		c.PureJSON(200, gin.H{"ok": true})
	}, validators...)
}

func (b *TestGinServerBuilder) WithPostPingRequiredAuthRoute(path string, validators ...ActorValidator) *TestGinServerBuilder {
	return b.WithRequiredAuthRoute(http.MethodPost, path, func(c *gin.Context, a A) {
		b.pingCounter++
		c.PureJSON(200, gin.H{"ok": true})
	}, validators...)
}

func (b *TestGinServerBuilder) WithAdminAuthRoute(method, path string, handler HandlerFunc, validators ...ActorValidator) *TestGinServerBuilder {
	b.adminAuthRoutes = append(b.adminAuthRoutes, route{path: path, handler: handler, method: method, validators: validators})
	return b
}

func (b *TestGinServerBuilder) WithGetPingAdminAuthRoute(path string, validators ...ActorValidator) *TestGinServerBuilder {
	return b.WithAdminAuthRoute(http.MethodGet, path, func(c *gin.Context, a A) {
		b.pingCounter++
		c.PureJSON(200, gin.H{"ok": true})
	}, validators...)
}

func (b *TestGinServerBuilder) Build() TestSetup {
	if b.service == config.ServiceId("") {
		b.service = config.ServiceIdPublic
	}

	if b.cfg == nil {
		adminSigningKey := &config.KeyDataRandomBytes{}
		b.cfg = config.FromRoot(&config.Root{
			Public: config.ServicePublic{
				ServiceHttp: config.ServiceHttp{
					PortVal:    &config.StringValue{&config.StringValueDirect{Value: "8080"}},
					DomainVal:  "example.com",
					IsHttpsVal: false,
				},
				CookieVal: &config.CookieConfig{
					DomainVal: util.ToPtr("example.com"),
				},
				SessionTimeoutVal: &config.HumanDuration{Duration: 10 * time.Hour},

				XsrfRequestQueueDepthVal: util.ToPtr(5),
			},
			SystemAuth: config.SystemAuth{
				JwtSigningKey: &config.KeyShared{
					SharedKey: &config.KeyDataRandomBytes{},
				},
				GlobalAESKey: &config.KeyDataRandomBytes{},
				AdminUsers: config.AdminUsersList{
					&config.AdminUser{
						Username: "bobdole",
						Key: &config.KeyShared{
							SharedKey: adminSigningKey,
						},
					},
					&config.AdminUser{
						Username: "ronaldreagan",
						Key: &config.KeyShared{
							SharedKey: adminSigningKey,
						},
					},
				},
			},
		})
	}

	if b.db == nil {
		b.cfg, b.db = database.MustApplyBlankTestDbConfig(b.testName, b.cfg)
	}

	if b.r == nil {
		b.cfg, b.r = apredis2.MustApplyTestConfig(b.cfg)
		if b.r == nil {
			panic("redis is nil")
		}
	}

	e := encrypt.NewFakeEncryptService(true)

	auth := NewService(b.cfg, b.cfg.MustGetService(b.service).(config.HttpService), b.db, b.r, e, test_utils.NewTestLogger())

	if len(b.defaultValidators) > 0 {
		auth = auth.WithDefaultActorValidators(b.defaultValidators...)
	}
	b.ginEngine = gin.New()

	for _, r := range b.openRoutes {
		b.ginEngine.Handle(r.method, r.path, func(gctx *gin.Context) { r.handler(gctx, auth) })
	}

	for _, r := range b.optionalAuthRoutes {
		b.ginEngine.Handle(r.method, r.path, auth.Optional(r.validators...), func(gctx *gin.Context) { r.handler(gctx, auth) })
	}

	for _, r := range b.optionalXsrfNotRequiredAuthRoutes {
		b.ginEngine.Handle(r.method, r.path, auth.OptionalXsrfNotRequired(r.validators...), func(gctx *gin.Context) { r.handler(gctx, auth) })
	}

	for _, r := range b.requiredAuthRoutes {
		b.ginEngine.Handle(r.method, r.path, auth.Required(r.validators...), func(gctx *gin.Context) { r.handler(gctx, auth) })
	}

	for _, r := range b.adminAuthRoutes {
		b.ginEngine.Handle(r.method, r.path, auth.AdminOnly(r.validators...), func(gctx *gin.Context) { r.handler(gctx, auth) })
	}

	return TestSetup{
		pingCounter: &b.pingCounter,
		Service:     b.service,
		Gin:         b.ginEngine,
		Cfg:         b.cfg,
		Db:          b.db,
		R:           b.r,
		CookieJar:   util.Must(cookiejar.New(nil)),
	}
}

type TestSetup struct {
	pingCounter *int
	Service     config.ServiceId
	Gin         *gin.Engine
	Cfg         config.C
	Db          database.DB
	R           apredis.Client
	CookieJar   http.CookieJar
	XSRFToken   string
}

// MustGetValidAdminUser gives an admin user that can sign JWTs. This method makes sure the admin exists in the database
// regardless of if they have interacted with the system previously.
func (ts *TestSetup) MustGetValidAdminUser(ctx context.Context) database.Actor {
	a, err := ts.Db.GetActorByExternalId(ctx, "admin/bobdole")
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		panic(err)
	}

	if errors.Is(err, database.ErrNotFound) {
		a = &database.Actor{
			ID:         uuid.New(),
			ExternalId: "admin/bobdole",
			Email:      "bobdole@example.com",
			Admin:      true,
		}
		if err := ts.Db.CreateActor(ctx, a); err != nil {
			panic(err)
		}
	}

	return *a
}

// MustGetValidUninitializedAdminUser give a valid admin, but does not create a user in the database ahead of time.
func (ts *TestSetup) MustGetValidUninitializedAdminUser(ctx context.Context) database.Actor {
	return database.Actor{
		ID:         uuid.New(),
		ExternalId: "admin/ronaldreagan",
		Email:      "ronaldreagan@example.com",
		Admin:      true,
	}
}

// MustGetValidUser gives an user that can sign JWTs. This method makes sure the admin exists in the database
// regardless of if they have interacted with the system previously.
func (ts *TestSetup) MustGetValidUserByExternalId(ctx context.Context, externalId string) database.Actor {
	a, err := ts.Db.GetActorByExternalId(ctx, externalId)
	if err != nil && !errors.Is(err, database.ErrNotFound) {
		panic(err)
	}

	if errors.Is(err, database.ErrNotFound) {
		a = &database.Actor{
			ID:         uuid.New(),
			ExternalId: externalId,
			Email:      "jimmycarter@example.com",
		}
		if err := ts.Db.CreateActor(ctx, a); err != nil {
			panic(err)
		}
	}

	return *a
}

func (ts *TestSetup) MustGetValidUser(ctx context.Context) database.Actor {
	return ts.MustGetValidUserByExternalId(ctx, "jimmycarter")
}

func (ts *TestSetup) GetPingCount() int {
	return *ts.pingCounter
}

func (ts *TestSetup) MustGetValidSigningTokenForUser() config.Key {
	return ts.Cfg.GetRoot().SystemAuth.JwtSigningKey
}

func (ts *TestSetup) MustGetValidSigningTokenForAdmin() config.Key {
	return ts.Cfg.GetRoot().SystemAuth.AdminUsers.All()[0].Key
}

func (ts *TestSetup) GET(ctx context.Context, path string) (responseJson gin.H, statusCode int, debugHeader string) {
	return ts.GetWithSigner(ctx, path, func(req *http.Request) {})
}

func (ts *TestSetup) GetWithSigner(ctx context.Context, path string, sign func(req *http.Request)) (responseJson gin.H, statusCode int, debugHeader string) {
	return ts.Request(ctx, http.MethodGet, path, nil, sign)
}

func (ts *TestSetup) POST(ctx context.Context, path string, body gin.H) (responseJson gin.H, statusCode int, debugHeader string) {
	return ts.Request(ctx, http.MethodPost, path, body, nil)
}

func (ts *TestSetup) Request(ctx context.Context, method string, path string, body gin.H, sign func(req *http.Request)) (responseJson gin.H, statusCode int, debugHeader string) {
	var bodyReader io.Reader
	if body != nil {
		// Convert the body into a JSON reader
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create a request to the specified path
	req, _ := http.NewRequestWithContext(ctx, method, path, bodyReader)
	w := httptest.NewRecorder()

	if ts.XSRFToken != "" {
		req.Header.Set(xsrfHeaderKey, ts.XSRFToken)
	}

	// Retrieve cookies for the request URL from the cookie jar
	cookies := ts.CookieJar.Cookies(util.Must(url.Parse("http://example.com")))
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	if sign != nil {
		sign(req)
	}

	// Use the Gin engine to handle the request
	ts.Gin.ServeHTTP(w, req)

	resp := w.Result()
	ts.CookieJar.SetCookies(util.Must(url.Parse("http://example.com")), resp.Cookies())

	// Read the XSRF token from the response headers and store it
	if xsrfToken := resp.Header.Get(xsrfHeaderKey); xsrfToken != "" {
		ts.XSRFToken = xsrfToken
	}

	// Decode the response JSON into an Actor object
	if err := json.Unmarshal(w.Body.Bytes(), &responseJson); err != nil {
		panic(err)
	}

	// Return the response actor and HTTP status code
	return responseJson, w.Code, w.Header().Get(api_common.DebugHeader)
}
func (ts *TestSetup) PostWithSigner(ctx context.Context, path string, body gin.H, sign func(req *http.Request)) (responseJson gin.H, statusCode int, debugHeader string) {
	// Convert the body into a JSON reader
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	bodyReader := bytes.NewReader(bodyBytes)

	// Create a request to the specified path
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, path, bodyReader)
	w := httptest.NewRecorder()

	sign(req)

	// Use the Gin engine to handle the request
	ts.Gin.ServeHTTP(w, req)

	// Decode the response JSON into an Actor object
	if err := json.Unmarshal(w.Body.Bytes(), &responseJson); err != nil {
		panic(err)
	}

	// Return the response actor and HTTP status code
	return responseJson, w.Code, w.Header().Get(api_common.DebugHeader)
}

// MustGetInvalidAdminUser gives an admin user that cannot be used to sign JWTs as it is not listed in the config
// admin user list. This admin does not actually exist in the database, the database actor is just used to pass
// the information.
func (ts *TestSetup) MustGetInvalidAdminUser(ctx context.Context) database.Actor {
	return database.Actor{
		ID:         uuid.New(),
		ExternalId: "admin/billclinton",
		Email:      "billclinton@example.com",
		Admin:      true,
	}
}

func actorIsBobDole(actor *database.Actor) (bool, string) {
	if actor.ExternalId == "bobdole" {
		return true, ""
	}

	return false, "invalid actor external id"
}

func TestAuth(t *testing.T) {
	t.Setenv("AUTHPROXY_DEBUG_MODE", "true")

	ctx := context.Background()
	t.Run("unauthenticated", func(t *testing.T) {
		t.Run("open route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingOpenRoute("/ping").
				Build()

			resp, statusCode, debugHeader := ts.GET(ctx, "/ping")
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())
		})
		t.Run("optional auth route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingOptionalAuthRoute("/ping").
				Build()

			resp, statusCode, debugHeader := ts.GET(ctx, "/ping")
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())
		})
		t.Run("optional xsrf not required auth route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingOptionalXsrfNotRequiredAuthRoute("/ping").
				Build()

			resp, statusCode, debugHeader := ts.GET(ctx, "/ping")
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())
		})
		t.Run("required auth route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingRequiredAuthRoute("/ping").
				Build()

			resp, statusCode, debugHeader := ts.GET(ctx, "/ping")
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, gin.H{"error": "Unauthorized"}, resp)
			require.Equal(t, 0, ts.GetPingCount())
		})
	})
	t.Run("jwt query param auth", func(t *testing.T) {
		t.Run("normal actor", func(t *testing.T) {
			t.Run("open route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOpenRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithServiceId(ts.Service).
					WithActorId(ts.MustGetValidUser(ctx).ExternalId).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))
				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())
			})
			t.Run("optional auth route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOptionalAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUser(ctx).ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())
			})
			t.Run("optional auth route with default validator", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOptionalAuthRoute("/ping").
					WithDefaultValidator(actorIsBobDole).
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "bobdole").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())

				s = jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "jimmycarter").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader = ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusForbidden, statusCode, debugHeader)
				require.Equal(t, gin.H{"error": "invalid actor external id"}, resp)
				require.Equal(t, 1, ts.GetPingCount()) // Not incremented
			})
			t.Run("optional auth route with validator", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOptionalAuthRoute("/ping", actorIsBobDole).
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "bobdole").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())

				s = jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "jimmycarter").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader = ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusForbidden, statusCode, debugHeader)
				require.Equal(t, gin.H{"error": "invalid actor external id"}, resp)
				require.Equal(t, 1, ts.GetPingCount()) // Not incremented
			})
			t.Run("required auth route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingRequiredAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUser(ctx).ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))
				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())
			})
			t.Run("required auth route with default validator", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingRequiredAuthRoute("/ping").
					WithDefaultValidator(actorIsBobDole).
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "bobdole").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())

				s = jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "jimmycarter").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader = ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusForbidden, statusCode, debugHeader)
				require.Equal(t, gin.H{"error": "invalid actor external id"}, resp)
				require.Equal(t, 1, ts.GetPingCount()) // Not incremented
			})
			t.Run("required auth route with validator", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingRequiredAuthRoute("/ping", actorIsBobDole).
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "bobdole").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())

				s = jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUserByExternalId(ctx, "jimmycarter").ExternalId).
					WithServiceId(ts.Service).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader = ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusForbidden, statusCode, debugHeader)
				require.Equal(t, gin.H{"error": "invalid actor external id"}, resp)
				require.Equal(t, 1, ts.GetPingCount()) // Not incremented
			})
		})
		t.Run("admin actor", func(t *testing.T) {
			t.Run("open route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOpenRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithServiceId(ts.Service).
					WithActorId(ts.MustGetValidAdminUser(ctx).ExternalId).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForAdmin()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))
				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())
			})
			t.Run("optional auth route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOptionalAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithServiceId(ts.Service).
					WithActorId(ts.MustGetValidAdminUser(ctx).ExternalId).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForAdmin()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))

				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())
			})
			t.Run("required auth route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingRequiredAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithServiceId(ts.Service).
					WithActorId(ts.MustGetValidAdminUser(ctx).ExternalId).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForAdmin()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))
				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())
			})
			t.Run("required auth route invalid admin", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingRequiredAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithServiceId(ts.Service).
					WithActorId(ts.MustGetInvalidAdminUser(ctx).ExternalId).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForAdmin()).
					MustSignerCtx(ctx)

				_, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))
				require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
				require.Equal(t, 0, ts.GetPingCount())
			})
			t.Run("required auth route uninitialized admin", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingRequiredAuthRoute("/ping").
					Build()

				// Even though we don't send the actor for the admin, the system should initialize the admin user
				// because this admin is listed as valid in the system.
				s := jwt.NewJwtTokenBuilder().
					WithServiceId(ts.Service).
					WithActorId(ts.MustGetValidUninitializedAdminUser(ctx).ExternalId).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForAdmin()).
					MustSignerCtx(ctx)

				resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/ping"))
				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, gin.H{"ok": true}, resp)
				require.Equal(t, 1, ts.GetPingCount())
			})
		})
	})
	t.Run("jwt header auth", func(t *testing.T) {
		t.Run("open route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingOpenRoute("/ping").
				Build()

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			resp, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())
		})
		t.Run("optional auth route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingOptionalAuthRoute("/ping").
				Build()

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			resp, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)

			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())
		})
		t.Run("optional xsrf not required auth route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingOptionalXsrfNotRequiredAuthRoute("/ping").
				Build()

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			resp, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)

			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())
		})
		t.Run("required auth route", func(t *testing.T) {
			ts := NewTestGinServerBuilder(t.Name()).
				WithGetPingRequiredAuthRoute("/ping").
				Build()

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			resp, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())
		})
	})
	t.Run("invalid jwt", func(t *testing.T) {
		t.Run("invalid audience", func(t *testing.T) {
			t.Run("open route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOpenRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithAudience("invalid").
					WithActorId(ts.MustGetValidUser(ctx).ExternalId).
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				_, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)
				require.Equal(t, http.StatusOK, statusCode, debugHeader)
				require.Equal(t, 1, ts.GetPingCount())
			})
			t.Run("optional auth route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOptionalAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUser(ctx).ExternalId).
					WithAudience("invalid").
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				_, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)

				require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
				require.Equal(t, 0, ts.GetPingCount())
			})
			t.Run("optional xsrf not required auth route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingOptionalXsrfNotRequiredAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUser(ctx).ExternalId).
					WithAudience("invalid").
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				_, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)

				require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
				require.Equal(t, 0, ts.GetPingCount())
			})
			t.Run("required auth route", func(t *testing.T) {
				ts := NewTestGinServerBuilder(t.Name()).
					WithGetPingRequiredAuthRoute("/ping").
					Build()

				s := jwt.NewJwtTokenBuilder().
					WithActorId(ts.MustGetValidUser(ctx).ExternalId).
					WithAudience("invalid").
					MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
					MustSignerCtx(ctx)

				_, statusCode, debugHeader := ts.GetWithSigner(ctx, "/ping", s.SignAuthHeader)
				require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
				require.Equal(t, 0, ts.GetPingCount())
			})
		})
	})
	t.Run("session", func(t *testing.T) {
		setup := func(t *testing.T) TestSetup {
			return NewTestGinServerBuilder(t.Name()).
				WithRequiredAuthRoute(http.MethodGet, "/initiate-session", func(gctx *gin.Context, auth A) {
					ra := GetAuthFromGinContext(gctx)
					err := auth.EstablishGinSession(gctx, ra)
					if err != nil {
						api_common.NewHttpStatusErrorBuilder().
							WithInternalErr(err).
							BuildStatusError().
							WriteGinResponse(api_common.NewMockDebuggable(true), gctx)
						return
					}

					gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
				}).
				WithRequiredAuthRoute(http.MethodGet, "/end-session", func(gctx *gin.Context, auth A) {
					ra := GetAuthFromGinContext(gctx)
					err := auth.EndGinSession(gctx, ra)
					if err != nil {
						api_common.NewHttpStatusErrorBuilder().
							WithInternalErr(err).
							BuildStatusError().
							WriteGinResponse(api_common.NewMockDebuggable(true), gctx)
						return
					}

					gctx.PureJSON(http.StatusOK, gin.H{"ok": true})
				}).
				WithGetPingRequiredAuthRoute("/ping-get").
				WithPostPingRequiredAuthRoute("/ping-post").
				Build()
		}

		t.Run("full flow", func(t *testing.T) {
			ts := setup(t)

			// No session
			resp, statusCode, debugHeader := ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 0, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 0, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.GET(ctx, "/initiate-session")
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)

			resp, statusCode, debugHeader = ts.GET(ctx, "/end-session")
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			// Start session
			resp, statusCode, debugHeader = ts.GET(ctx, s.SignUrlQuery("/initiate-session"))
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)

			resp, statusCode, debugHeader = ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 2, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.GET(ctx, "/end-session")
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 2, ts.GetPingCount())

			// No session again
			resp, statusCode, debugHeader = ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 2, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 2, ts.GetPingCount())
		})

		t.Run("requires xsrf for post but not for get", func(t *testing.T) {
			ts := setup(t)

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			// Start session
			resp, statusCode, debugHeader := ts.GET(ctx, s.SignUrlQuery("/initiate-session"))
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)

			ts.XSRFToken = ""

			resp, statusCode, debugHeader = ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusOK, statusCode, debugHeader) // Does not require xsrf
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())

			ts.XSRFToken = ""

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusForbidden, statusCode, debugHeader) // Requires xsrf
			require.Equal(t, 1, ts.GetPingCount())
		})
	})
	t.Run("session initiate via post", func(t *testing.T) {
		setup := func(t *testing.T) TestSetup {
			return NewTestGinServerBuilder(t.Name()).
				WithOptionalXsrfNotRequiredAuthRoute(http.MethodPost, "/initiate-session", func(gctx *gin.Context, auth A) {
					ra := GetAuthFromGinContext(gctx)
					if ra.IsAuthenticated() {
						err := auth.EstablishGinSession(gctx, ra)
						if err != nil {
							api_common.NewHttpStatusErrorBuilder().
								WithInternalErr(err).
								BuildStatusError().
								WriteGinResponse(api_common.NewMockDebuggable(true), gctx)
							return
						}

						gctx.PureJSON(http.StatusOK, gin.H{"ok": true, "session": true})
					} else {
						gctx.PureJSON(http.StatusOK, gin.H{"ok": true, "session": false})
					}
				}).
				WithOptionalXsrfNotRequiredAuthRoute(http.MethodPost, "/end-session", func(gctx *gin.Context, auth A) {
					ra := GetAuthFromGinContext(gctx)
					if ra.IsAuthenticated() {
						err := auth.EndGinSession(gctx, ra)
						if err != nil {
							api_common.NewHttpStatusErrorBuilder().
								WithInternalErr(err).
								BuildStatusError().
								WriteGinResponse(api_common.NewMockDebuggable(true), gctx)
							return
						}

						gctx.PureJSON(http.StatusOK, gin.H{"ok": true, "terminated": true})
					} else {
						gctx.PureJSON(http.StatusOK, gin.H{"ok": true, "terminated": false})
					}
				}).
				WithGetPingRequiredAuthRoute("/ping-get").
				WithPostPingRequiredAuthRoute("/ping-post").
				Build()
		}

		t.Run("full flow", func(t *testing.T) {
			ts := setup(t)

			// No session
			resp, statusCode, debugHeader := ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 0, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 0, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/initiate-session", gin.H{})
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true, "session": false}, resp)

			resp, statusCode, debugHeader = ts.POST(ctx, "/end-session", gin.H{})
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true, "terminated": false}, resp)

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			// Start session (note that this is a post, but it does not require xsrf)
			resp, statusCode, debugHeader = ts.POST(ctx, s.SignUrlQuery("/initiate-session"), gin.H{})
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true, "session": true}, resp)

			resp, statusCode, debugHeader = ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 2, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/end-session", gin.H{})
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true, "terminated": true}, resp)
			require.Equal(t, 2, ts.GetPingCount())

			// No session again
			resp, statusCode, debugHeader = ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 2, ts.GetPingCount())

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusUnauthorized, statusCode, debugHeader)
			require.Equal(t, 2, ts.GetPingCount())
		})

		t.Run("requires xsrf for post but not for get", func(t *testing.T) {
			ts := setup(t)

			s := jwt.NewJwtTokenBuilder().
				WithActorId(ts.MustGetValidUser(ctx).ExternalId).
				WithServiceId(ts.Service).
				MustWithConfigKey(ctx, ts.MustGetValidSigningTokenForUser()).
				MustSignerCtx(ctx)

			// Start session (note no XSRF required for the post)
			resp, statusCode, debugHeader := ts.POST(ctx, s.SignUrlQuery("/initiate-session"), gin.H{})
			require.Equal(t, http.StatusOK, statusCode, debugHeader)
			require.Equal(t, gin.H{"ok": true, "session": true}, resp)

			ts.XSRFToken = ""

			resp, statusCode, debugHeader = ts.GET(ctx, "/ping-get")
			require.Equal(t, http.StatusOK, statusCode, debugHeader) // Does not require xsrf
			require.Equal(t, gin.H{"ok": true}, resp)
			require.Equal(t, 1, ts.GetPingCount())

			ts.XSRFToken = ""

			resp, statusCode, debugHeader = ts.POST(ctx, "/ping-post", gin.H{})
			require.Equal(t, http.StatusForbidden, statusCode, debugHeader) // Requires xsrf
			require.Equal(t, 1, ts.GetPingCount())
		})
	})
}
