package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	test_clock "k8s.io/utils/clock/testing"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var (
	testJwtValid            = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJyZW1hcms0MiIsImF1ZCI6WyJ0ZXN0X3N5cyJdLCJleHAiOjI3ODkxOTE4MjIsIm5iZiI6MTUyNjg4NDIyMiwiaWF0IjoxNzI3NzQwODAwLCJqdGkiOiJyYW5kb20gaWQiLCJ1c2VyIjp7ImlkIjoiaWQxIiwiaXAiOiIxMjcuMC4wLjEiLCJlbWFpbCI6Im1lQGV4YW1wbGUuY29tIiwiYXR0cnMiOnsiYm9vbGEiOnRydWUsInN0cmEiOiJzdHJhLXZhbCJ9fX0.atOMrivB3LYIQeBEb-R28T9-TFezyvkYscxPC2jbfV4"
	testJwtValidNoHandshake = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJyZW1hcms0MiIsImF1ZCI6WyJ0ZXN0X3N5cyJdLCJleHAiOjI3ODkxOTE4MjIsIm5iZiI6MTUyNjg4NDIyMiwiaWF0IjoxNzI3NzQwODAwLCJqdGkiOiJyYW5kb20gaWQiLCJ1c2VyIjp7ImlkIjoiaWQxIiwiaXAiOiIxMjcuMC4wLjEiLCJlbWFpbCI6Im1lQGV4YW1wbGUuY29tIiwiYXR0cnMiOnsiYm9vbGEiOnRydWUsInN0cmEiOiJzdHJhLXZhbCJ9fX0.atOMrivB3LYIQeBEb-R28T9-TFezyvkYscxPC2jbfV4"
	testJwtValidSess        = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJyZW1hcms0MiIsImF1ZCI6WyJ0ZXN0X3N5cyJdLCJleHAiOjI3ODkxOTE4MjIsIm5iZiI6MTUyNjg4NDIyMiwiaWF0IjoxNzI3NzQwODAwLCJqdGkiOiJyYW5kb20gaWQiLCJ1c2VyIjp7ImlkIjoiaWQxIiwiaXAiOiIxMjcuMC4wLjEiLCJlbWFpbCI6Im1lQGV4YW1wbGUuY29tIiwiYXR0cnMiOnsiYm9vbGEiOnRydWUsInN0cmEiOiJzdHJhLXZhbCJ9fSwic2Vzc19vbmx5Ijp0cnVlfQ.dTB_PamolW5w7LFRBbXDuN_SKh9BOMawVH_6ECaWsvE"
	testJwtExpired          = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE1MjY4ODc4MjIsImp0aSI6InJhbmRvbSBpZCIs" +
		"ImlzcyI6InJlbWFyazQyIiwibmJmIjoxNTI2ODg0MjIyLCJ1c2VyIjp7Im5hbWUiOiJuYW1lMSIsImlkIjoiaWQxIiwicGljdHVyZSI6IiI" +
		"sImFkbWluIjpmYWxzZX0sInN0YXRlIjoiMTIzNDU2IiwiZnJvbSI6ImZyb20ifQ.4_dCrY9ihyfZIedz-kZwBTxmxU1a52V7IqeJrOqTzE4"
	testJwtBadSign    = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJ0ZXN0X3N5cyIsImV4cCI6Mjc4OTE5MTgyMiwianRpIjoicmFuZG9tIGlkIiwiaXNzIjoicmVtYXJrNDIiLCJuYmYiOjE1MjY4ODQyMjIsInVzZXIiOnsibmFtZSI6Im5hbWUxIiwiaWQiOiJpZDEiLCJwaWN0dXJlIjoiaHR0cDovL2V4YW1wbGUuY29tL3BpYy5wbmciLCJpcCI6IjEyNy4wLjAuMSIsImVtYWlsIjoibWVAZXhhbXBsZS5jb20iLCJhdHRycyI6eyJib29sYSI6dHJ1ZSwic3RyYSI6InN0cmEtdmFsIn19LCJoYW5kc2hha2UiOnsic3RhdGUiOiIxMjM0NTYiLCJmcm9tIjoiZnJvbSIsImlkIjoibXlpZC0xMjM0NTYifX0.PRuys_Ez2QWhAMp3on4Xpdc5rebKcL7-HGncvYsdYns"
	testJwtNbf        = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJyZW1hcms0MiIsImF1ZCI6WyJ0ZXN0X3N5cyJdLCJleHAiOjI3ODkxOTE4MjIsIm5iZiI6Mjc0NTUzNjQ2MSwianRpIjoicmFuZG9tIGlkIiwidXNlciI6eyJpZCI6ImlkMSIsImlwIjoiMTI3LjAuMC4xIiwiZW1haWwiOiJtZUBleGFtcGxlLmNvbSJ9fQ.q6nzbIT5DnbSWIXS8Vt5l1BoFptzmOInPccmIRgFdXo"
	testJwtNoneAlg    = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJpc3MiOiJodHRwczovL2p3dC1pZHAuZXhhbXBsZS5jb20iLCJzdWIiOiJtYWlsdG86bWlrZUBleGFtcGxlLmNvbSIsIm5iZiI6MTU0Njc0MzcxMSwiZXhwIjoxNTQ2NzQ3MzExLCJpYXQiOjE1NDY3NDM3MTEsImp0aSI6ImlkMTIzNDU2IiwidHlwIjoiaHR0cHM6Ly9leGFtcGxlLmNvbS9yZWdpc3RlciJ9."
	testJwtNoAud      = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjI3ODkxOTE4MjIsImp0aSI6InJhbmRvbSBpZCIsImlzcyI6InJlbWFyazQyIiwibmJmIjoxNTI2ODg0MjIyLCJ1c2VyIjp7Im5hbWUiOiJuYW1lMSIsImlkIjoiaWQxIiwicGljdHVyZSI6Imh0dHA6Ly9leGFtcGxlLmNvbS9waWMucG5nIiwiaXAiOiIxMjcuMC4wLjEiLCJlbWFpbCI6Im1lQGV4YW1wbGUuY29tIiwiYXR0cnMiOnsiYm9vbGEiOnRydWUsInN0cmEiOiJzdHJhLXZhbCJ9fSwiaGFuZHNoYWtlIjp7InN0YXRlIjoiMTIzNDU2IiwiZnJvbSI6ImZyb20iLCJpZCI6Im15aWQtMTIzNDU2In19.pzRsCcZjH7MItUvnBmyGv74Qg3qx8vCGmsZP6lF_Z9A"
	testJwtValidAud   = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJ0ZXN0X2F1ZF9vbmx5IiwiZXhwIjoyNzg5MTkxODIyLCJqdGkiOiJyYW5kb20gaWQiLCJpc3MiOiJyZW1hcms0MiIsIm5iZiI6MTUyNjg4NDIyMiwidXNlciI6eyJuYW1lIjoibmFtZTEiLCJpZCI6ImlkMSIsInBpY3R1cmUiOiJodHRwOi8vZXhhbXBsZS5jb20vcGljLnBuZyIsImlwIjoiMTI3LjAuMC4xIiwiZW1haWwiOiJtZUBleGFtcGxlLmNvbSIsImF0dHJzIjp7ImJvb2xhIjp0cnVlLCJzdHJhIjoic3RyYS12YWwifX0sImhhbmRzaGFrZSI6eyJzdGF0ZSI6IjEyMzQ1NiIsImZyb20iOiJmcm9tIiwiaWQiOiJteWlkLTEyMzQ1NiJ9fQ.Ll3uS2jvj_yYZms43_w6zJOdkDR305M4AiFVLXnSd7Y"
	testJwtNonAudSign = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJ0ZXN0X2F1ZF9vbmx5IiwiZXhwIjoyNzg5MTkxODIyLCJqdGkiOiJyYW5kb20gaWQiLCJpc3MiOiJyZW1hcms0MiIsIm5iZiI6MTUyNjg4NDIyMiwidXNlciI6eyJuYW1lIjoibmFtZTEiLCJpZCI6ImlkMSIsInBpY3R1cmUiOiJodHRwOi8vZXhhbXBsZS5jb20vcGljLnBuZyIsImlwIjoiMTI3LjAuMC4xIiwiZW1haWwiOiJtZUBleGFtcGxlLmNvbSIsImF0dHJzIjp7ImJvb2xhIjp0cnVlLCJzdHJhIjoic3RyYS12YWwifX0sImhhbmRzaGFrZSI6eyJzdGF0ZSI6IjEyMzQ1NiIsImZyb20iOiJmcm9tIiwiaWQiOiJteWlkLTEyMzQ1NiJ9fQ.kJc-U970h3j9riUhFLR9vN_YCUQwZ66tjk7zdC9OiUg"
)

func mockKeyStore(aud string) (string, error) {
	if aud == "test_aud_only" {
		return "audsecret", nil
	}
	return "xyz 12345", nil
}

func TestJWT_Token(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := testClaims
	res, err := j.Token(context.Background(), &claims)
	assert.Nil(t, err)
	assert.Equal(t, testJwtValid, res)
}

func TestJWT_Parse(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{Config: cfg, ServiceId: config.ServiceIdAdminApi})
	claims, err := j.Parse(testContext, testJwtValid)
	assert.NoError(t, err)
	assert.False(t, j.IsExpired(testContext, claims))
	assert.Equal(t, &Actor{ID: "id1", IP: "127.0.0.1",
		Email: "me@example.com", Attributes: map[string]interface{}{"boola": true, "stra": "stra-val"}}, claims.Actor)

	claims, err = j.Parse(testContext, testJwtExpired)
	assert.EqualError(t, err, "can't parse token: token has invalid claims: token is expired")

	_, err = j.Parse(testContext, testJwtNbf)
	assert.EqualError(t, err, "can't parse token: token has invalid claims: token is not valid yet")

	_, err = j.Parse(testContext, "bad")
	assert.NotNil(t, err, "bad token")

	_, err = j.Parse(testContext, testJwtBadSign)
	assert.EqualError(t, err, "can't parse token: token signature is invalid: signature is invalid")

	_, err = j.Parse(testContext, testJwtNoneAlg)
	assert.EqualError(t, err, "can't parse token: token is unverifiable: error while executing keyfunc: unexpected signing method: none")

	j = NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})
	_, err = j.Parse(testContext, testJwtValid)
	assert.NotNil(t, err, "bad token", "valid token parsed with wrong secret")

	j = NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})
	_, err = j.Parse(testContext, testJwtValid)
	assert.EqualError(t, err, "can't get secret: err blah")

}

func TestJWT_Set(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)

	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := testClaims

	rr := httptest.NewRecorder()
	c, err := j.Set(testContext, rr, &claims)
	assert.Nil(t, err)
	assert.Equal(t, claims, c)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	assert.Equal(t, jwtCookieName, cookies[0].Name)
	assert.Equal(t, testJwtValidNoHandshake, cookies[0].Value)
	assert.Equal(t, int(testConfig.SystemAuth.CookieDurationVal.Seconds()), cookies[0].MaxAge)
	assert.Equal(t, xsrfCookieName, cookies[1].Name)
	assert.Equal(t, "random id", cookies[1].Value)

	claims.SessionOnly = true
	rr = httptest.NewRecorder()
	_, err = j.Set(testContext, rr, &claims)
	assert.Nil(t, err)
	cookies = rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	assert.Equal(t, jwtCookieName, cookies[0].Name)
	assert.Equal(t, testJwtValidSess, cookies[0].Value)
	assert.Equal(t, 0, cookies[0].MaxAge)
	assert.Equal(t, xsrfCookieName, cookies[1].Name)
	assert.Equal(t, "random id", cookies[1].Value)
	assert.Equal(t, "example.com", cookies[0].Domain)

	rr = httptest.NewRecorder()

	// Check below looks at issued at changing, so we need a different time than the test context
	_, err = j.Set(context.Background().WithClock(test_clock.NewFakeClock(time.Date(2024, 11, 2, 0, 0, 0, 0, time.UTC))), rr, &claims)
	assert.Nil(t, err)
	cookies = rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	assert.Equal(t, jwtCookieName, cookies[0].Name)
	assert.NotEqual(t, testJwtValidSess, cookies[0].Value, "iat changed the token")
	assert.Equal(t, "", rr.Result().Header.Get(jwtHeaderKey), "no JWT header set")
}

func TestJWT_SetWithDomain(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config: cfg,
	})

	claims := testClaims

	rr := httptest.NewRecorder()
	c, err := j.Set(testContext, rr, &claims)
	assert.Nil(t, err)
	assert.Equal(t, claims, c)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 2, len(cookies))
	assert.Equal(t, jwtCookieName, cookies[0].Name)
	assert.Equal(t, "example.com", cookies[0].Domain)
	assert.Equal(t, testJwtValidNoHandshake, cookies[0].Value)
	assert.Equal(t, int(testConfig.SystemAuth.CookieDuration().Seconds()), cookies[0].MaxAge)
	assert.Equal(t, xsrfCookieName, cookies[1].Name)
	assert.Equal(t, "random id", cookies[1].Value)

}

func TestJWT_SendJWTHeader(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		SendJWTHeader: true,
		Config:        cfg,
		ServiceId:     config.ServiceIdAdminApi,
	})

	rr := httptest.NewRecorder()
	_, err := j.Set(testContext, rr, &testClaims)
	assert.Nil(t, err)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	require.Equal(t, 0, len(cookies), "no cookies set")
	assert.Equal(t, testJwtValid, rr.Result().Header.Get(jwtHeaderKey))
}

func TestJWT_SetProlonged(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config:    cfg,
		ServiceId: config.ServiceIdAdminApi,
	})

	claims := testClaims
	claims.ExpiresAt = nil

	rr := httptest.NewRecorder()
	_, err := j.Set(testContext, rr, &claims)
	assert.NoError(t, err)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	assert.Equal(t, jwtCookieName, cookies[0].Name)

	cc, err := j.Parse(testContext, cookies[0].Value)
	assert.NoError(t, err)
	assert.True(t, cc.ExpiresAt.Time.After(testContext.Clock().Now()))
}

func TestJWT_NoIssuer(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config: cfg,
	})

	claims := testClaims
	claims.Issuer = ""

	rr := httptest.NewRecorder()
	_, err := j.Set(testContext, rr, &claims)
	assert.NoError(t, err)
	cookies := rr.Result().Cookies()
	t.Log(cookies)
	assert.Equal(t, jwtCookieName, cookies[0].Name)

	cc, err := j.Parse(testContext, cookies[0].Value)
	assert.NoError(t, err)
	assert.Equal(t, cfg.GetRoot().SystemAuth.JwtIssuer(), cc.Issuer)
}

func TestJWT_GetFromHeader(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config: cfg,
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add(jwtHeaderKey, testJwtValid)
	claims, token, err := j.Get(testContext, req)
	assert.Nil(t, err)
	assert.Equal(t, testJwtValid, token)
	assert.False(t, j.IsExpired(testContext, claims))
	assert.Equal(t, &Actor{ID: "id1", IP: "127.0.0.1",
		Email: "me@example.com", Audience: []string{"test_sys"},
		Attributes: map[string]interface{}{"boola": true, "stra": "stra-val"}}, claims.Actor)
	assert.Equal(t, "remark42", claims.Issuer)

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Add(jwtHeaderKey, testJwtExpired)
	_, _, err = j.Get(testContext, req)
	assert.NotNil(t, err)

	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Add(jwtHeaderKey, "bad bad token")
	_, _, err = j.Get(testContext, req)
	require.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "failed to get token: can't parse token: token is malformed: token contains an invalid number of segments"), err.Error())
}

func TestJWT_GetFromQuery(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config: cfg,
	})

	req := httptest.NewRequest("GET", "/blah?jwt="+testJwtValid, nil)
	claims, token, err := j.Get(testContext, req)
	assert.NoError(t, err)
	assert.Equal(t, testJwtValid, token)
	assert.False(t, j.IsExpired(testContext, claims))
	assert.Equal(t, &Actor{ID: "id1", IP: "127.0.0.1",
		Email: "me@example.com", Audience: []string{"test_sys"},
		Attributes: map[string]interface{}{"boola": true, "stra": "stra-val"}}, claims.Actor)
	assert.Equal(t, "remark42", claims.Issuer)

	req = httptest.NewRequest("GET", "/blah?token="+testJwtExpired, nil)
	_, _, err = j.Get(testContext, req)
	assert.NotNil(t, err)

	req = httptest.NewRequest("GET", "/blah?jwt=blah", nil)
	_, _, err = j.Get(testContext, req)
	require.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "failed to get token: can't parse token: token is malformed: token contains an invalid number of segments"), err.Error())
}

func TestJWT_GetFailed(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{Config: cfg})
	req := httptest.NewRequest("GET", "/", nil)
	_, _, err := j.Get(testContext, req)
	assert.Error(t, err, "token cookie was not presented")
}

func TestJWT_SetAndGetWithCookies(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config: cfg,
	})

	claims := testClaims
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
	assert.Equal(t, 200, resp.StatusCode)

	req := httptest.NewRequest("GET", "/valid", nil)
	req.AddCookie(resp.Cookies()[0])
	req.Header.Add(xsrfHeaderKey, "random id")
	r, _, err := j.Get(testContext, req)
	assert.Nil(t, err)
	assert.Equal(t, &Actor{ID: "id1", IP: "127.0.0.1",
		Email: "me@example.com", Audience: []string{"test_sys"},
		Attributes: map[string]interface{}{"boola": true, "stra": "stra-val"}}, r.Actor)
	assert.Equal(t, "remark42", claims.Issuer)
	assert.Equal(t, true, claims.SessionOnly)
	t.Log(resp.Cookies())
}

func TestJWT_SetAndGetWithXsrfMismatch(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config: cfg,
	})

	claims := testClaims
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
	assert.Equal(t, 200, resp.StatusCode)

	req := httptest.NewRequest("GET", "/valid", nil)
	req.AddCookie(resp.Cookies()[0])
	req.Header.Add(xsrfHeaderKey, "random id wrong")
	_, _, err = j.Get(testContext, req)
	assert.EqualError(t, err, "xsrf mismatch")

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

	assert.Equal(t, claims, c)
}

func TestJWT_SetAndGetWithCookiesExpired(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{
		Config: cfg,
	})

	claims := testClaims
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
	assert.Equal(t, 200, resp.StatusCode)

	req := httptest.NewRequest("GET", "/expired", nil)
	req.AddCookie(resp.Cookies()[0])
	req.Header.Add(xsrfHeaderKey, "random id")
	_, _, err = j.Get(testContext, req)
	assert.Error(t, err)
	assert.True(t, IsTokenExpiredError(err))
}

func TestJWT_Reset(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{Config: cfg})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/valid" {
			j.Reset(w)
			w.WriteHeader(200)
		}
	}))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/valid")
	require.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	assert.Equal(t, `auth-proxy-jwt=; Path=/; Domain=example.com; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Max-Age=0; SameSite=None`, resp.Header.Get("Set-Cookie"))
	assert.Equal(t, "0", resp.Header.Get("Content-Length"))
}

func TestJWT_Validator(t *testing.T) {
	ch := ValidatorFunc(func(token string, claims JwtTokenClaims) bool {
		return token == "good"
	})
	assert.True(t, ch.Validate("good", JwtTokenClaims{}))
	assert.False(t, ch.Validate("bad", JwtTokenClaims{}))
}

func TestClaims_String(t *testing.T) {
	s := testClaims.String()
	assert.True(t, strings.Contains(s, `"aud":["test_sys"]`))
	assert.True(t, strings.Contains(s, `"exp":2789191822`))
	assert.True(t, strings.Contains(s, `"jti":"random id"`))
	assert.True(t, strings.Contains(s, `"iss":"remark42"`))
	assert.True(t, strings.Contains(s, `"nbf":1526884222`))
	assert.True(t, strings.Contains(s, `"user":`))
}

func TestParseWithAud(t *testing.T) {
	cfg := config.ConfigFromRoot(&testConfig)
	j := NewService(Opts{AudSecrets: true, Config: cfg, ServiceId: config.ServiceIdAdminApi})

	claims, err := j.Parse(testContext, testJwtValid)
	assert.NoError(t, err)
	assert.False(t, j.IsExpired(testContext, claims))
	assert.Equal(t, &Actor{ID: "id1", IP: "127.0.0.1",
		Email: "me@example.com", Attributes: map[string]interface{}{"boola": true, "stra": "stra-val"}}, claims.Actor)

	claims, err = j.Parse(testContext, testJwtValidAud)
	assert.NoError(t, err)
	assert.Equal(t, jwt.ClaimStrings{"test_aud_only"}, claims.Audience)

	claims, err = j.Parse(testContext, testJwtNonAudSign)
	assert.EqualError(t, err, "can't parse token: token signature is invalid: signature is invalid")
}

func TestExtractTokenFromBearer(t *testing.T) {
	tok, err := extractTokenFromBearer("Bearer foo")
	assert.NoError(t, err)
	assert.Equal(t, "foo", tok)

	tok, err = extractTokenFromBearer("Bearer ")
	assert.NoError(t, err)
	assert.Equal(t, "", tok)
}

var testContext = context.Background().WithClock(test_clock.NewFakeClock(time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)))

var testClaims = JwtTokenClaims{
	RegisteredClaims: jwt.RegisteredClaims{
		ID:        "random id",
		Issuer:    "remark42",
		Audience:  []string{"test_sys"},
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
var testConfig = config.Root{
	SystemAuth: config.SystemAuth{
		CookieDurationVal:   10 * time.Hour,
		CookieDomain:        "example.com",
		JwtTokenDurationVal: 12 * time.Hour,
		JwtIssuerVal:        "example",
	},
	AdminApi: config.ApiHost{
		Port: 8080,
	},
}
