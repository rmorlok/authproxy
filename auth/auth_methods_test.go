package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/mohae/deepcopy"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	jwt2 "github.com/rmorlok/authproxy/jwt"
	"github.com/rmorlok/authproxy/test_utils"
	"github.com/rmorlok/authproxy/util"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
	test_clock "k8s.io/utils/clock/testing"
)

func TestAuth_Token(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), nil, nil, nil, test_utils.NewTestLogger())

	res, err := j.Token(testContext, testClaims())
	require.NoError(t, err)

	claims, err := j.Parse(testContext, res)
	require.NoError(t, err)
	require.NotNil(t, testClaims().Actor.ID, claims.Actor.ID)
}

func TestAuth_RoundtripGlobaleAESKey(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), nil, nil, nil, test_utils.NewTestLogger())

	claims := jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Issuer:    "remark42",
			Audience:  []string{string(config.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Actor: &jwt2.Actor{
			ID:    "id1",
			IP:    "127.0.0.1",
			Email: "me@example.com",
		},
	}

	t.Run("via service methods", func(t *testing.T) {
		tok, err := j.Token(testContext, &claims)
		require.NoError(t, err)
		rtClaims, err := j.Parse(testContext, tok)
		require.NoError(t, err)
		require.Equal(t, claims.Actor.ID, rtClaims.Actor.ID)

		tokRunes := []rune(tok)
		if len(tokRunes) >= 10 {
			tokRunes[9] = 'X' // Replace the 10th character (0-based index 9)
		}
		tok = string(tokRunes)
		_, err = j.Parse(testContext, tok)
		require.Error(t, err)
	})
	t.Run("via token builder", func(t *testing.T) {
		// Clone
		copiedClaims := deepcopy.Copy(&claims).(*jwt2.AuthProxyClaims)
		copiedClaims.SelfSigned = true

		tb := jwt2.NewJwtTokenBuilder().WithSecretKey(util.Must(cfg.GetRoot().SystemAuth.GlobalAESKey.GetData(testContext)))
		tok, err := tb.WithClaims(copiedClaims).TokenCtx(testContext)
		require.NoError(t, err)
		rtClaims, err := j.Parse(testContext, tok)
		require.NoError(t, err)
		require.Equal(t, claims.Actor.ID, rtClaims.Actor.ID)

		tokRunes := []rune(tok)
		if len(tokRunes) >= 10 {
			tokRunes[9] = 'X' // Replace the 10th character (0-based index 9)
		}
		tok = string(tokRunes)
		_, err = j.Parse(testContext, tok)
		require.Error(t, err)
	})
}

func TestAuth_RoundtripPublicPrivate(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), nil, nil, nil, test_utils.NewTestLogger())

	claims := jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Issuer:    "remark42",
			Audience:  []string{string(config.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Actor: &jwt2.Actor{
			ID:    "id1",
			IP:    "127.0.0.1",
			Email: "me@example.com",
		},
	}

	tok, err := j.Token(testContext, &claims)
	require.NoError(t, err)
	rtClaims, err := j.Parse(testContext, tok)
	require.NoError(t, err)
	require.Equal(t, claims.Actor.ID, rtClaims.Actor.ID)

	tokRunes := []rune(tok)
	if len(tokRunes) >= 10 {
		tokRunes[9] = 'X' // Replace the 10th character (0-based index 9)
	}
	tok = string(tokRunes)
	_, err = j.Parse(testContext, tok)
	require.Error(t, err)
}

func TestAuth_SecretKey(t *testing.T) {
	cfg := config.FromRoot(&testConfigSecretKey)
	j := NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), nil, nil, nil, test_utils.NewTestLogger())

	claims := jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Issuer:    "remark42",
			Audience:  []string{string(config.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Actor: &jwt2.Actor{
			ID:    "id7",
			IP:    "127.0.0.1",
			Email: "me@example.com",
		},
	}

	tb, err := jwt2.NewJwtTokenBuilder().WithConfigKey(testContext, cfg.GetRoot().SystemAuth.JwtSigningKey)
	require.NoError(t, err)

	tok, err := tb.WithClaims(&claims).TokenCtx(testContext)
	require.NoError(t, err)

	rtClaims, err := j.Parse(testContext, tok)
	require.NoError(t, err)
	require.Equal(t, claims.Actor.ID, rtClaims.Actor.ID)

	tokRunes := []rune(tok)
	if len(tokRunes) >= 10 {
		tokRunes[9] = 'X' // Replace the 10th character (0-based index 9)
	}
	tok = string(tokRunes)
	_, err = j.Parse(testContext, tok)
	require.Error(t, err)

}

func TestAuth_Parse(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), nil, nil, nil, test_utils.NewTestLogger())
	t.Run("valid", func(t *testing.T) {
		tok, err := j.Token(testContext, testClaims())
		require.NoError(t, err)

		claims, err := j.Parse(testContext, tok)
		require.NoError(t, err)
		require.False(t, claims.IsExpired(testContext))
		require.Equal(t, testClaims().Actor.Email, claims.Actor.Email)

	})
	t.Run("expired", func(t *testing.T) {
		org := jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(config.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
			},

			Actor: &jwt2.Actor{
				ID:    "id1",
				IP:    "127.0.0.1",
				Email: "me@example.com",
			},
		}

		tok, err := j.Token(testContext, &org)
		require.NoError(t, err)

		futureCtx := apctx.
			NewBuilderBackground().
			WithClock(test_clock.NewFakeClock(time.Date(2059, 10, 1, 0, 0, 0, 0, time.UTC))).
			Build()

		_, err = j.Parse(futureCtx, tok)
		require.Contains(t, err.Error(), "token is expired")
	})

	t.Run("not before", func(t *testing.T) {
		org := jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(config.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
			},

			Actor: &jwt2.Actor{
				ID:    "id1",
				IP:    "127.0.0.1",
				Email: "me@example.com",
			},
		}

		tok, err := j.Token(testContext, &org)
		require.NoError(t, err)

		pastCtx := apctx.
			NewBuilderBackground().
			WithClock(test_clock.NewFakeClock(time.Date(2017, 10, 1, 0, 0, 0, 0, time.UTC))).
			Build()

		_, err = j.Parse(pastCtx, tok)
		require.Contains(t, err.Error(), "token is not valid yet")
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := j.Parse(testContext, "bad")
		require.NotNil(t, err, "bad token")
	})

	t.Run("invalid signature", func(t *testing.T) {
		serv1 := j
		tb, err := jwt2.NewJwtTokenBuilder().WithConfigKey(testContext, cfg.GetRoot().SystemAuth.JwtSigningKey)
		require.NoError(t, err)

		tokServ1, err := tb.WithClaims(testClaims()).TokenCtx(testContext)
		require.NoError(t, err)

		// Valid with the current
		_, err = serv1.Parse(testContext, tokServ1)
		require.NoError(t, err)

		cfg := config.FromRoot(&config.Root{
			SystemAuth: config.SystemAuth{
				JwtTokenDurationVal: 12 * time.Hour,
				JwtIssuerVal:        "example",
				JwtSigningKey: &config.KeyPublicPrivate{
					PublicKey: &config.KeyDataFile{
						Path: "../test_data/system_keys/other-system.pub",
					},
					PrivateKey: &config.KeyDataFile{
						Path: "../test_data/system_keys/other-system",
					},
				},
			},
			AdminApi: config.ServiceAdminApi{
				ServiceHttp: config.ServiceHttp{
					PortVal: &config.StringValue{&config.StringValueDirect{Value: "8080"}},
				},
			},
		})
		serv2 := NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), nil, nil, nil, test_utils.NewTestLogger())

		tb2, err := jwt2.NewJwtTokenBuilder().WithConfigKey(testContext, cfg.GetRoot().SystemAuth.JwtSigningKey)
		require.NoError(t, err)

		tokServ2, err := tb2.WithClaims(testClaims()).TokenCtx(testContext)
		require.NoError(t, err)

		// Valid with the current
		_, err = serv2.Parse(testContext, tokServ2)
		require.NoError(t, err)

		// Reject cross system tokens
		_, err = serv1.Parse(testContext, tokServ2)
		require.Error(t, err)
		_, err = serv2.Parse(testContext, tokServ1)
		require.Error(t, err)

		require.NotNil(t, err, "bad token")
	})

	t.Run("admin", func(t *testing.T) {
		cfg := config.FromRoot(&config.Root{
			SystemAuth: config.SystemAuth{
				AdminUsers: &config.AdminUsersList{
					&config.AdminUser{
						Username: "bobdole",
						Key: &config.KeyPublicPrivate{
							PublicKey: &config.KeyDataFile{
								Path: "../test_data/admin_user_keys/bobdole.pub",
							},
						},
					},
				},
				JwtTokenDurationVal: 12 * time.Hour,
				JwtIssuerVal:        "example",
				JwtSigningKey: &config.KeyPublicPrivate{
					PublicKey: &config.KeyDataFile{
						Path: "../test_data/system_keys/other-system.pub",
					},
					PrivateKey: &config.KeyDataFile{
						Path: "../test_data/system_keys/other-system",
					},
				},
			},
			AdminApi: config.ServiceAdminApi{
				ServiceHttp: config.ServiceHttp{
					PortVal: &config.StringValue{&config.StringValueDirect{Value: "8080"}},
				},
			},
		})
		adminSrv := NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), nil, nil, nil, test_utils.NewTestLogger())

		t.Run("valid", func(t *testing.T) {
			token, err := jwt2.NewJwtTokenBuilder().
				WithActorId("bobdole").
				WithActorEmail("bobdole@example.com").
				WithPrivateKeyPath("../test_data/admin_user_keys/bobdole").
				WithAdmin().
				WithAudience(string(config.ServiceIdAdminApi)).
				TokenCtx(testContext)
			require.NoError(t, err)

			claims, err := adminSrv.Parse(testContext, token)
			require.NoError(t, err)
			require.Equal(t, "admin/bobdole", claims.Subject)
			require.True(t, claims.IsAdmin())
		})

		t.Run("unknown admin", func(t *testing.T) {
			token, err := jwt2.NewJwtTokenBuilder().
				WithActorId("billclinton").
				WithActorEmail("billclinton@example.com").
				WithPrivateKeyPath("../test_data/admin_user_keys/billclinton").
				WithAdmin().
				TokenCtx(testContext)
			require.NoError(t, err)

			_, err = adminSrv.Parse(testContext, token)
			require.Error(t, err)
		})

		t.Run("wrong key for admin", func(t *testing.T) {
			token, err := jwt2.NewJwtTokenBuilder().
				WithActorId("bobdole").
				WithActorEmail("bobdole@example.com").
				WithPrivateKeyPath("../test_data/admin_user_keys/billclinton").
				WithAdmin().
				TokenCtx(testContext)
			require.NoError(t, err)

			_, err = adminSrv.Parse(testContext, token)
			require.Error(t, err)
		})
	})
}

func TestAuth_establishAuthFromRequest(t *testing.T) {
	var a A
	var raw *service
	var db database.DB

	setup := func(t *testing.T) {
		cfg := config.FromRoot(&testConfigPublicPrivateKey)
		cfg, db = database.MustApplyBlankTestDbConfig(t.Name(), cfg)
		a = NewService(cfg, cfg.MustGetService(config.ServiceIdAdminApi).(config.HttpService), db, nil, nil, test_utils.NewTestLogger())
		raw = a.(*service)
	}

	t.Run("from header", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			t.Run("create actor", func(t *testing.T) {
				setup(t)

				tok, err := a.Token(testContext, testClaims())
				require.NoError(t, err)

				req := httptest.NewRequest("GET", "/", nil)
				req.Header.Add(JwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
				w := httptest.NewRecorder()
				ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
				require.NoError(t, err)
				require.True(t, ra.IsAuthenticated())
				require.Equal(t, testClaims().Actor.ID, ra.MustGetActor().ExternalId)

				actor, err := db.GetActorByExternalId(testContext, testClaims().Actor.ID)
				require.NoError(t, err)
				require.Equal(t, testClaims().Actor.ID, actor.ExternalId)
			})

			t.Run("actor loaded from database", func(t *testing.T) {
				setup(t)

				dbActorId := uuid.New()
				dbActor := &database.Actor{
					ID:         dbActorId,
					ExternalId: testClaims().Actor.ID,
					Email:      testClaims().Actor.Email,
				}
				require.NoError(t, db.CreateActor(testContext, dbActor))

				claims := *testClaims()
				claims.Actor = nil // Explicitly don't specify actor details

				tok, err := a.Token(testContext, &claims)
				require.NoError(t, err)

				req := httptest.NewRequest("GET", "/", nil)
				req.Header.Add(JwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
				w := httptest.NewRecorder()
				ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
				require.NoError(t, err)
				require.True(t, ra.IsAuthenticated())
				require.Equal(t, testClaims().Actor.ID, ra.MustGetActor().ExternalId)
			})

			t.Run("actor updated in database", func(t *testing.T) {
				setup(t)

				dbActorId := uuid.New()
				dbActor := &database.Actor{
					ID:         dbActorId,
					ExternalId: testClaims().Actor.ID,
					Email:      "old-" + testClaims().Actor.Email,
				}
				require.NoError(t, db.CreateActor(testContext, dbActor))

				tok, err := a.Token(testContext, testClaims())
				require.NoError(t, err)

				req := httptest.NewRequest("GET", "/", nil)
				req.Header.Add(JwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
				w := httptest.NewRecorder()
				ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
				require.NoError(t, err)
				require.True(t, ra.IsAuthenticated())
				require.Equal(t, testClaims().Actor.ID, ra.MustGetActor().ExternalId)
				require.Equal(t, testClaims().Actor.Email, ra.MustGetActor().Email)
			})
		})

		t.Run("expired", func(t *testing.T) {
			setup(t)

			tok, err := a.Token(testContext, testClaims())
			require.NoError(t, err)

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Add(JwtHeaderKey, tok)

			futureCtx := apctx.
				NewBuilderBackground().
				WithClock(test_clock.NewFakeClock(time.Date(2059, 10, 1, 0, 0, 0, 0, time.UTC))).
				Build()

			w := httptest.NewRecorder()
			_, err = raw.establishAuthFromRequest(futureCtx, true, req, w)
			require.NotNil(t, err)

			actor, err := db.GetActorByExternalId(testContext, testClaims().Actor.ID)
			require.NoError(t, err)
			require.Nil(t, actor)
		})

		t.Run("bad token", func(t *testing.T) {
			setup(t)

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Add(JwtHeaderKey, "Bearer: bad bad token")
			w := httptest.NewRecorder()
			_, err := raw.establishAuthFromRequest(testContext, true, req, w)
			require.NotNil(t, err)

			actor, err := db.GetActorByExternalId(testContext, testClaims().Actor.ID)
			require.NoError(t, err)
			require.Nil(t, actor)
		})
	})
	t.Run("from query", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			setup(t)

			tok, err := a.Token(testContext, testClaims())
			require.NoError(t, err)

			req := httptest.NewRequest("GET", "/blah?auth_token="+tok, nil)
			w := httptest.NewRecorder()
			ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
			require.NoError(t, err)
			require.True(t, ra.IsAuthenticated())

			require.Equal(t, ra.MustGetActor().ID, ra.MustGetActor().ID)
		})
		t.Run("expired", func(t *testing.T) {
			setup(t)

			tok, err := a.Token(testContext, testClaims())
			require.NoError(t, err)

			futureCtx := apctx.
				NewBuilderBackground().
				WithClock(test_clock.NewFakeClock(time.Date(2059, 10, 1, 0, 0, 0, 0, time.UTC))).
				Build()

			req := httptest.NewRequest("GET", "/blah?auth_token="+tok, nil)
			w := httptest.NewRecorder()
			_, err = raw.establishAuthFromRequest(futureCtx, true, req, w)
			require.NotNil(t, err)
		})
		t.Run("bad token", func(t *testing.T) {
			setup(t)

			req := httptest.NewRequest("GET", "/blah?auth_token=blah", nil)
			w := httptest.NewRecorder()
			_, err := raw.establishAuthFromRequest(testContext, true, req, w)
			require.NotNil(t, err)
		})
	})
}

func TestAuth_Nonce(t *testing.T) {
	now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
	ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

	type TestSetup struct {
		Gin      *gin.Engine
		Cfg      config.C
		AuthUtil *AuthTestUtil
	}

	setup := func(t *testing.T) *TestSetup {
		cfg := config.FromRoot(&testConfigPublicPrivateKey)
		cfg, auth, authUtil := TestAuthService(t, config.ServiceIdAdminApi, cfg)
		r := gin.Default()
		r.GET("/", auth.Required(), func(c *gin.Context) {
			ra := MustGetAuthFromGinContext(c)
			c.String(200, util.ToPtr(ra.MustGetActor()).ExternalId)
		})

		return &TestSetup{
			Gin:      r,
			Cfg:      cfg,
			AuthUtil: authUtil,
		}
	}

	t.Run("valid", func(t *testing.T) {
		ts := setup(t)
		c := testClaims()
		c.Nonce = util.ToPtr(uuid.New())
		c.ExpiresAt = &jwt.NumericDate{now.Add(time.Hour)}
		c.NotBefore = nil

		tok, err := ts.AuthUtil.s.Token(testContext, c)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?auth_token="+tok, nil).WithContext(ctx)
		ts.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, c.Actor.ID, w.Body.String())
	})

	t.Run("expired", func(t *testing.T) {
		ts := setup(t)
		c := testClaims()
		c.Nonce = util.ToPtr(uuid.New())
		c.ExpiresAt = &jwt.NumericDate{now.Add(-time.Hour)}
		c.NotBefore = nil

		tok, err := ts.AuthUtil.s.Token(testContext, c)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?auth_token="+tok, nil).WithContext(ctx)
		ts.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("used more than once", func(t *testing.T) {
		ts := setup(t)
		c := testClaims()
		c.Nonce = util.ToPtr(uuid.New())
		c.ExpiresAt = &jwt.NumericDate{now.Add(time.Hour)}
		c.NotBefore = nil

		tok, err := ts.AuthUtil.s.Token(testContext, c)
		require.NoError(t, err)

		// First request ok
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?auth_token="+tok, nil).WithContext(ctx)
		ts.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, c.Actor.ID, w.Body.String())

		// Second request fail
		w = httptest.NewRecorder()
		req = httptest.NewRequest("GET", "/?auth_token="+tok, nil).WithContext(ctx)
		ts.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("token does not contain expiry", func(t *testing.T) {
		ts := setup(t)
		c := testClaims()
		c.Nonce = util.ToPtr(uuid.New())
		c.ExpiresAt = nil
		c.NotBefore = nil

		tok, err := ts.AuthUtil.s.Token(testContext, c)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/?auth_token="+tok, nil).WithContext(ctx)
		ts.Gin.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestClaims_String(t *testing.T) {
	s := testClaims().String()
	require.True(t, strings.Contains(s, `"aud":["admin-api"]`))
	require.True(t, strings.Contains(s, `"exp":2789191822`))
	require.True(t, strings.Contains(s, `"jti":"random id"`))
	require.True(t, strings.Contains(s, `"iss":"remark42"`))
	require.True(t, strings.Contains(s, `"nbf":1526884222`))
	require.True(t, strings.Contains(s, `"actor":`))
}

func TestExtractTokenFromBearer(t *testing.T) {
	tok, err := extractTokenFromBearer("Bearer foo")
	require.NoError(t, err)
	require.Equal(t, "foo", tok)

	tok, err = extractTokenFromBearer("Bearer ")
	require.NoError(t, err)
	require.Equal(t, "", tok)
}

var testContext = apctx.
	NewBuilderBackground().
	WithClock(test_clock.NewFakeClock(time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC))).
	Build()

func testClaims() *jwt2.AuthProxyClaims {
	return &jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Subject:   "id1",
			Issuer:    "remark42",
			Audience:  []string{string(config.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Actor: &jwt2.Actor{
			ID:    "id1",
			IP:    "127.0.0.1",
			Email: "me@example.com",
		},
	}
}

var testConfigPublicPrivateKey = config.Root{
	SystemAuth: config.SystemAuth{
		JwtTokenDurationVal: 12 * time.Hour,
		JwtIssuerVal:        "example",
		JwtSigningKey: &config.KeyPublicPrivate{
			PublicKey: &config.KeyDataFile{
				Path: "../test_data/system_keys/system.pub",
			},
			PrivateKey: &config.KeyDataFile{
				Path: "../test_data/system_keys/system",
			},
		},
		AdminUsers: config.AdminUsersList{
			&config.AdminUser{
				Username: "aid1",
				Key: &config.KeyPublicPrivate{
					PublicKey: &config.KeyDataFile{
						Path: "../test_data/system_keys/system.pub",
					},
					PrivateKey: &config.KeyDataFile{
						Path: "../test_data/system_keys/system",
					},
				},
			},
		},
		GlobalAESKey: &config.KeyDataBase64Val{
			Base64: "tOqE5HtiujnwB7pXt6lQLH8/gCh6TmMq9uSLFtJxZtU=",
		},
	},
	AdminApi: config.ServiceAdminApi{
		ServiceHttp: config.ServiceHttp{
			PortVal: &config.StringValue{&config.StringValueDirect{Value: "8080"}},
		},
	},
}

var testConfigSecretKey = config.Root{
	SystemAuth: config.SystemAuth{
		JwtTokenDurationVal: 12 * time.Hour,
		JwtIssuerVal:        "example",
		JwtSigningKey: &config.KeyShared{
			SharedKey: &config.KeyDataBase64Val{
				Base64: "+xKbTv+pdvWK+4ucIsUcAHqzEhelLWuud80+fy1pQzc=",
			},
		},
		GlobalAESKey: &config.KeyDataBase64Val{
			Base64: "tOqE5HtiujnwB7pXt6lQLH8/gCh6TmMq9uSLFtJxZtU=",
		},
	},
	AdminApi: config.ServiceAdminApi{
		ServiceHttp: config.ServiceHttp{
			PortVal: &config.StringValue{&config.StringValueDirect{Value: "8080"}},
		},
	},
}
