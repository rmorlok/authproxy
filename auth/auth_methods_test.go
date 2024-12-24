package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/stretchr/testify/require"
	test_clock "k8s.io/utils/clock/testing"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var (
	testJwtValidSess = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJyZW1hcms0MiIsImF1ZCI6WyJ0ZXN0X3N5cyJdLCJleHAiOjI3ODkxOTE4MjIsIm5iZiI6MTUyNjg4NDIyMiwiaWF0IjoxNzI3NzQwODAwLCJqdGkiOiJyYW5kb20gaWQiLCJ1c2VyIjp7ImlkIjoiaWQxIiwiaXAiOiIxMjcuMC4wLjEiLCJlbWFpbCI6Im1lQGV4YW1wbGUuY29tIiwiYXR0cnMiOnsiYm9vbGEiOnRydWUsInN0cmEiOiJzdHJhLXZhbCJ9fSwic2Vzc19vbmx5Ijp0cnVlfQ.dTB_PamolW5w7LFRBbXDuN_SKh9BOMawVH_6ECaWsvE"
)

func TestJWT_Token(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	res, err := j.Token(testContext, testClaims())
	require.NoError(t, err)

	claims, err := j.Parse(testContext, res)
	require.NoError(t, err)
	require.NotNil(t, testClaims().Actor.ID, claims.Actor.ID)
}

func TestJWT_SendJWTHeader(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		SendJWTHeader: true,
		Config:        cfg,
		ServiceId:     config.ServiceIdAdminApi,
	})

	rr := httptest.NewRecorder()
	_, err := j.Set(testContext, rr, testClaims())
	require.Nil(t, err)
	cookies := rr.Result().Cookies()
	require.Equal(t, 0, len(cookies), "no cookies set")
	token := strings.Replace(rr.Result().Header.Get(jwtHeaderKey), "Bearer ", "", 1)
	claims, err := j.Parse(testContext, token)
	require.NoError(t, err)
	require.NotNil(t, testClaims().Actor.ID, claims.Actor.ID)
}

func TestJWT_RoundtripPublicPrivate(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})

	claims := JwtTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Issuer:    "remark42",
			Audience:  []string{string(config.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{testContext.Clock().Now()},
		},

		Actor: &Actor{
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

func TestJWT_RoundtripSecretKey(t *testing.T) {
	cfg := config.FromRoot(&testConfigSecretKey)
	j := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})

	claims := JwtTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Issuer:    "remark42",
			Audience:  []string{string(config.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{testContext.Clock().Now()},
		},

		Actor: &Actor{
			ID:    "id7",
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

func TestJWT_Parse(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})
	t.Run("valid", func(t *testing.T) {
		tok, err := j.Token(testContext, testClaims())
		require.NoError(t, err)

		claims, err := j.Parse(testContext, tok)
		require.NoError(t, err)
		require.False(t, j.IsExpired(testContext, claims))
		require.Equal(t, testClaims().Actor.Email, claims.Actor.Email)

	})
	t.Run("expired", func(t *testing.T) {
		org := JwtTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(config.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{testContext.Clock().Now()},
			},

			Actor: &Actor{
				ID:    "id1",
				IP:    "127.0.0.1",
				Email: "me@example.com",
			},
		}

		tok, err := j.Token(testContext, &org)
		require.NoError(t, err)

		futureCtx := context.
			Background().
			WithClock(test_clock.NewFakeClock(time.Date(2059, 10, 1, 0, 0, 0, 0, time.UTC)))

		_, err = j.Parse(futureCtx, tok)
		require.Contains(t, err.Error(), "token is expired")
	})

	t.Run("not before", func(t *testing.T) {
		org := JwtTokenClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Issuer:    "remark42",
				Audience:  []string{string(config.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{testContext.Clock().Now()},
			},

			Actor: &Actor{
				ID:    "id1",
				IP:    "127.0.0.1",
				Email: "me@example.com",
			},
		}

		tok, err := j.Token(testContext, &org)
		require.NoError(t, err)

		pastCtx := context.
			Background().
			WithClock(test_clock.NewFakeClock(time.Date(2017, 10, 1, 0, 0, 0, 0, time.UTC)))

		_, err = j.Parse(pastCtx, tok)
		require.Contains(t, err.Error(), "token is not valid yet")
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := j.Parse(testContext, "bad")
		require.NotNil(t, err, "bad token")
	})

	t.Run("invalid signature", func(t *testing.T) {
		serv1 := j
		tokServ1, err := serv1.Token(testContext, testClaims())
		require.NoError(t, err)

		// Valid with the current
		_, err = serv1.Parse(testContext, tokServ1)
		require.NoError(t, err)

		cfg := config.FromRoot(&config.Root{
			SystemAuth: config.SystemAuth{
				CookieDurationVal:   10 * time.Hour,
				CookieDomain:        "example.com",
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
			AdminApi: config.ApiHost{
				Port: 8080,
			},
		})
		serv2 := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})

		tokServ2, err := serv2.Token(testContext, testClaims())
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
				CookieDurationVal:   10 * time.Hour,
				CookieDomain:        "example.com",
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
			AdminApi: config.ApiHost{
				Port: 8080,
			},
		})
		adminSrv := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})

		t.Run("valid", func(t *testing.T) {
			token, err := NewJwtTokenBuilder().
				WithActorId("bobdole").
				WithActorEmail("bobdole@example.com").
				WithPrivateKeyPath("../test_data/admin_user_keys/bobdole").
				WithAdmin().
				WithAudience(string(config.ServiceIdAdminApi)).
				TokenCtx(testContext)
			require.NoError(t, err)

			claims, err := adminSrv.Parse(testContext, token)
			require.NoError(t, err)
			require.Equal(t, "admin/bobdole", claims.Actor.ID)
			require.True(t, claims.Actor.IsAdmin())
		})

		t.Run("unknown admin", func(t *testing.T) {
			token, err := NewJwtTokenBuilder().
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
			token, err := NewJwtTokenBuilder().
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

func TestJWT_Set(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)

	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := *testClaims()

	rr := httptest.NewRecorder()
	c, err := j.Set(testContext, rr, &claims)
	require.Nil(t, err)
	require.Equal(t, claims, *c)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	require.Equal(t, jwtCookieName, cookies[0].Name)

	returnedClaims, err := j.Parse(testContext, cookies[0].Value)
	require.NoError(t, err)
	require.Equal(t, testClaims().Actor.ID, returnedClaims.Actor.ID)

	require.Equal(t, int(testConfigPublicPrivateKey.SystemAuth.CookieDurationVal.Seconds()), cookies[0].MaxAge)
	require.Equal(t, xsrfCookieName, cookies[1].Name)
	require.Equal(t, "random id", cookies[1].Value)

	claims.SessionOnly = true
	rr = httptest.NewRecorder()
	_, err = j.Set(testContext, rr, &claims)
	require.Nil(t, err)
	cookies = rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	require.Equal(t, jwtCookieName, cookies[0].Name)

	returnedClaims, err = j.Parse(testContext, cookies[0].Value)
	require.NoError(t, err)
	require.Equal(t, testClaims().Actor.ID, returnedClaims.Actor.ID)

	require.Equal(t, 0, cookies[0].MaxAge)
	require.Equal(t, xsrfCookieName, cookies[1].Name)
	require.Equal(t, "random id", cookies[1].Value)
	require.Equal(t, "example.com", cookies[0].Domain)

	rr = httptest.NewRecorder()

	// Check below looks at issued at changing, so we need a different time than the test context
	_, err = j.Set(context.Background().WithClock(test_clock.NewFakeClock(time.Date(2024, 11, 2, 0, 0, 0, 0, time.UTC))), rr, &claims)
	require.Nil(t, err)
	cookies = rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	require.Equal(t, jwtCookieName, cookies[0].Name)
	require.NotEqual(t, testJwtValidSess, cookies[0].Value, "iat changed the token")
	require.Equal(t, "", rr.Result().Header.Get(jwtHeaderKey), "no JWT header set")
}

func TestJWT_SetWithDomain(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := *testClaims()

	rr := httptest.NewRecorder()
	_, err := j.Set(testContext, rr, &claims)
	require.Nil(t, err)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	require.Equal(t, jwtCookieName, cookies[0].Name)
	require.Equal(t, "example.com", cookies[0].Domain)

	returnedClaims, err := j.Parse(testContext, cookies[0].Value)
	require.NoError(t, err)
	require.Equal(t, testClaims().Actor.ID, returnedClaims.Actor.ID)

	require.Equal(t, int(testConfigPublicPrivateKey.SystemAuth.CookieDuration().Seconds()), cookies[0].MaxAge)
	require.Equal(t, xsrfCookieName, cookies[1].Name)
	require.Equal(t, "random id", cookies[1].Value)
}

func TestJWT_SetProlonged(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := *testClaims()
	claims.ExpiresAt = nil

	rr := httptest.NewRecorder()
	_, err := j.Set(testContext, rr, &claims)
	require.NoError(t, err)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, jwtCookieName, cookies[0].Name)

	cc, err := j.Parse(testContext, cookies[0].Value)
	require.NoError(t, err)
	require.True(t, cc.ExpiresAt.Time.After(testContext.Clock().Now()))
}

func TestJWT_NoIssuer(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := *testClaims()
	claims.Issuer = ""

	rr := httptest.NewRecorder()
	_, err := j.Set(testContext, rr, &claims)
	require.NoError(t, err)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, jwtCookieName, cookies[0].Name)

	cc, err := j.Parse(testContext, cookies[0].Value)
	require.NoError(t, err)
	require.Equal(t, string(config.ServiceIdAdminApi), cc.Issuer)
}

func TestJWT_GetFromHeader(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	t.Run("valid", func(t *testing.T) {
		tok, err := j.Token(testContext, testClaims())
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Add(jwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
		claims, _, err := j.Get(testContext, req)
		require.Nil(t, err)
		require.Equal(t, testClaims().Actor.ID, claims.Actor.ID)
	})

	t.Run("expired", func(t *testing.T) {
		tok, err := j.Token(testContext, testClaims())
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Add(jwtHeaderKey, tok)

		futureCtx := context.
			Background().
			WithClock(test_clock.NewFakeClock(time.Date(2059, 10, 1, 0, 0, 0, 0, time.UTC)))

		_, _, err = j.Get(futureCtx, req)
		require.NotNil(t, err)
	})

	t.Run("bad token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Add(jwtHeaderKey, "Bearer: bad bad token")
		_, _, err := j.Get(testContext, req)
		require.NotNil(t, err)
	})
}

func TestJWT_GetFromQuery(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	t.Run("valid", func(t *testing.T) {
		tok, err := j.Token(testContext, testClaims())
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/blah?jwt="+tok, nil)
		claims, _, err := j.Get(testContext, req)
		require.NoError(t, err)

		require.Equal(t, claims.Actor.ID, claims.Actor.ID)
		require.False(t, j.IsExpired(testContext, claims))
	})
	t.Run("expired", func(t *testing.T) {
		tok, err := j.Token(testContext, testClaims())
		require.NoError(t, err)

		futureCtx := context.
			Background().
			WithClock(test_clock.NewFakeClock(time.Date(2059, 10, 1, 0, 0, 0, 0, time.UTC)))

		req := httptest.NewRequest("GET", "/blah?token="+tok, nil)
		_, _, err = j.Get(futureCtx, req)
		require.NotNil(t, err)
	})
	t.Run("bad token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/blah?jwt=blah", nil)
		_, _, err := j.Get(testContext, req)
		require.NotNil(t, err)
	})
}

func TestJWT_GetFailed(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})
	req := httptest.NewRequest("GET", "/", nil)
	_, _, err := j.Get(testContext, req)
	require.Error(t, err, "token cookie was not presented")
}

func TestJWT_SetAndGetWithCookies(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := *testClaims()
	claims.SessionOnly = true

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/valid" {
			_, e := j.Set(testContext, w, &claims)
			require.NoError(t, e)
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/valid")
	require.Nil(t, err)
	require.Equal(t, 200, resp.StatusCode)

	req := httptest.NewRequest("GET", "/valid", nil)
	req.AddCookie(resp.Cookies()[0])
	req.Header.Add(xsrfHeaderKey, "random id")
	r, _, err := j.Get(testContext, req)
	require.Nil(t, err)
	require.Equal(t, &Actor{ID: "id1", IP: "127.0.0.1",
		Email: "me@example.com", Audience: []string{"admin-api"}}, r.Actor)
	require.Equal(t, "admin-api", claims.Issuer)
	require.Equal(t, true, claims.SessionOnly)
	t.Log(resp.Cookies())
}

func TestJWT_SetAndGetWithXsrfMismatch(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := *testClaims()
	claims.SessionOnly = true
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/valid" {
			_, e := j.Set(testContext, w, &claims)
			require.NoError(t, e)
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/valid")
	require.Nil(t, err)
	require.Equal(t, 200, resp.StatusCode)

	req := httptest.NewRequest("GET", "/valid", nil)
	req.AddCookie(resp.Cookies()[0])
	req.Header.Add(xsrfHeaderKey, "random id wrong")
	_, _, err = j.Get(testContext, req)
	require.EqualError(t, err, "xsrf mismatch")

	cfg.GetRoot().SystemAuth.DisableXSRF = true
	req = httptest.NewRequest("GET", "/valid", nil)
	req.AddCookie(resp.Cookies()[0])
	req.Header.Add(xsrfHeaderKey, "random id wrong")
	c, _, err := j.Get(testContext, req)
	require.NoError(t, err, "xsrf mismatch, but ignored")
	claims.Actor.Audience = c.Audience // set aud to user because we don't do the normal Get call

	// Force UTC for check because parsing isn't
	c.ExpiresAt = jwt.NewNumericDate(c.ExpiresAt.UTC())
	c.IssuedAt = jwt.NewNumericDate(c.IssuedAt.UTC())
	c.NotBefore = jwt.NewNumericDate(c.NotBefore.UTC())

	require.Equal(t, claims, *c)
}

func TestJWT_SetAndGetWithCookiesExpired(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := *testClaims()
	claims.RegisteredClaims.ExpiresAt = &jwt.NumericDate{time.Date(2018, 5, 21, 1, 35, 22, 0, time.Local)}
	claims.RegisteredClaims.NotBefore = &jwt.NumericDate{time.Date(2018, 5, 21, 1, 30, 22, 0, time.Local)}
	claims.SessionOnly = true

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/expired" {
			_, e := j.Set(testContext, w, &claims)
			require.NoError(t, e)
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/expired")
	require.Nil(t, err)
	require.Equal(t, 200, resp.StatusCode)

	req := httptest.NewRequest("GET", "/expired", nil)
	req.AddCookie(resp.Cookies()[0])
	req.Header.Add(xsrfHeaderKey, "random id")
	_, _, err = j.Get(testContext, req)
	require.Error(t, err)
	require.True(t, IsTokenExpiredError(err))
}

func TestJWT_Reset(t *testing.T) {
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/valid" {
			j.Reset(w)
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/valid")
	require.Nil(t, err)
	require.Equal(t, 200, resp.StatusCode)

	require.Equal(t, `auth-proxy-jwt=; Path=/; Domain=example.com; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=0; SameSite=None`, resp.Header.Get("Set-Cookie"))
	require.Equal(t, "0", resp.Header.Get("Content-Length"))
}

func TestJWT_Validator(t *testing.T) {
	ch := ValidatorFunc(func(token string, claims JwtTokenClaims) bool {
		return token == "good"
	})
	require.True(t, ch.Validate("good", JwtTokenClaims{}))
	require.False(t, ch.Validate("bad", JwtTokenClaims{}))
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

var testContext = context.Background().WithClock(test_clock.NewFakeClock(time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)))

func testClaims() *JwtTokenClaims {
	return &JwtTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Issuer:    "remark42",
			Audience:  []string{string(config.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{testContext.Clock().Now()},
		},

		Actor: &Actor{
			ID:    "id1",
			IP:    "127.0.0.1",
			Email: "me@example.com",
		},
	}
}

var testConfigPublicPrivateKey = config.Root{
	SystemAuth: config.SystemAuth{
		CookieDurationVal:   10 * time.Hour,
		CookieDomain:        "example.com",
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
	},
	AdminApi: config.ApiHost{
		Port: 8080,
	},
}

var testConfigSecretKey = config.Root{
	SystemAuth: config.SystemAuth{
		CookieDurationVal:   10 * time.Hour,
		CookieDomain:        "example.com",
		JwtTokenDurationVal: 12 * time.Hour,
		JwtIssuerVal:        "example",
		JwtSigningKey: &config.KeyShared{
			SharedKey: &config.KeyDataBase64Val{
				Base64: "+xKbTv+pdvWK+4ucIsUcAHqzEhelLWuud80+fy1pQzc=",
			},
		},
	},
	AdminApi: config.ApiHost{
		Port: 8080,
	},
}
