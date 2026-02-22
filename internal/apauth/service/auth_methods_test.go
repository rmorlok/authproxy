package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/mohae/deepcopy"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	jwt2 "github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encrypt"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
	clock "k8s.io/utils/clock/testing"
	test_clock "k8s.io/utils/clock/testing"
)

func pathToTestData(path string) string {
	return "../../../test_data/" + path
}

func TestAuth_Token(t *testing.T) {
	t.Parallel()
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), nil, nil, nil, test_utils.NewTestLogger())

	res, err := j.Token(testContext, testClaims())
	require.NoError(t, err)

	claims, err := j.Parse(testContext, res)
	require.NoError(t, err)
	require.NotNil(t, testClaims().Actor.Id, claims.Actor.Id)
}

func TestAuth_RoundtripGlobaleAESKey(t *testing.T) {
	t.Parallel()
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), nil, nil, nil, test_utils.NewTestLogger())

	claims := jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Subject:   "id1",
			Issuer:    "remark42",
			Audience:  []string{string(sconfig.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Actor: &core.Actor{
			ExternalId: "id1",
			Namespace:  "root",
		},
	}

	t.Run("via service methods", func(t *testing.T) {
		tok, err := j.Token(testContext, &claims)
		require.NoError(t, err)
		rtClaims, err := j.Parse(testContext, tok)
		require.NoError(t, err)
		require.Equal(t, claims.Actor.Id, rtClaims.Actor.Id)

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
		require.Equal(t, claims.Actor.Id, rtClaims.Actor.Id)

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
	t.Parallel()
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), nil, nil, nil, test_utils.NewTestLogger())

	claims := jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Subject:   "id1",
			Issuer:    "remark42",
			Audience:  []string{string(sconfig.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Actor: &core.Actor{
			ExternalId: "id1",
			Namespace:  "root",
		},
	}

	tok, err := j.Token(testContext, &claims)
	require.NoError(t, err)
	rtClaims, err := j.Parse(testContext, tok)
	require.NoError(t, err)
	require.Equal(t, claims.Actor.Id, rtClaims.Actor.Id)

	tokRunes := []rune(tok)
	if len(tokRunes) >= 10 {
		tokRunes[9] = 'X' // Replace the 10th character (0-based index 9)
	}
	tok = string(tokRunes)
	_, err = j.Parse(testContext, tok)
	require.Error(t, err)
}

func TestAuth_SecretKey(t *testing.T) {
	t.Parallel()
	cfg := config.FromRoot(&testConfigSecretKey)
	ctrl := gomock.NewController(t)
	mockDb := mock.NewMockDB(ctrl)
	mockDb.
		EXPECT().
		GetActorByExternalId(gomock.Any(), "root", "external-id7").
		Return(
			// Actor that does not define a self-signing key
			&database.Actor{
				ExternalId: "external-id7",
				Namespace:  "root",
			},
			nil,
		)
	authService := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), mockDb, nil, nil, test_utils.NewTestLogger())

	claims := jwt2.AuthProxyClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "random id",
			Subject:   "external-id7",
			Issuer:    "remark42",
			Audience:  []string{string(sconfig.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Actor: &core.Actor{
			ExternalId: "external-id7",
			Namespace:  "root",
		},
	}

	tb, err := jwt2.NewJwtTokenBuilder().WithConfigKey(testContext, cfg.GetRoot().SystemAuth.JwtSigningKey)
	require.NoError(t, err)

	tok, err := tb.WithClaims(&claims).TokenCtx(testContext)
	require.NoError(t, err)

	rtClaims, err := authService.Parse(testContext, tok)
	require.NoError(t, err)
	require.Equal(t, claims.Actor.Id, rtClaims.Actor.Id)

	tokRunes := []rune(tok)
	if len(tokRunes) >= 10 {
		tokRunes[9] = 'X' // Replace the 10th character (0-based index 9)
	}
	tok = string(tokRunes)
	_, err = authService.Parse(testContext, tok)
	require.Error(t, err)

}

func TestAuth_Parse(t *testing.T) {
	t.Parallel()
	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	j := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), nil, nil, nil, test_utils.NewTestLogger())
	t.Run("valid", func(t *testing.T) {
		tok, err := j.Token(testContext, testClaims())
		require.NoError(t, err)

		claims, err := j.Parse(testContext, tok)
		require.NoError(t, err)
		require.False(t, claims.IsExpired(testContext))
		require.Equal(t, testClaims().Actor.ExternalId, claims.Actor.ExternalId)

	})
	t.Run("expired", func(t *testing.T) {
		org := jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Subject:   "id1",
				Issuer:    "remark42",
				Audience:  []string{string(sconfig.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
			},

			Actor: &core.Actor{
				ExternalId: "id1",
				Namespace:  "root",
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
				Subject:   "id1",
				Issuer:    "remark42",
				Audience:  []string{string(sconfig.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
			},

			Actor: &core.Actor{
				ExternalId: "id1",
				Namespace:  "root",
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

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				JwtTokenDurationVal: 12 * time.Hour,
				JwtIssuerVal:        "example",
				JwtSigningKey: &sconfig.Key{
					InnerVal: &sconfig.KeyPublicPrivate{
						PublicKey: &sconfig.KeyData{
							InnerVal: &sconfig.KeyDataFile{
								Path: pathToTestData("system_keys/other-system.pub"),
							},
						},
						PrivateKey: &sconfig.KeyData{
							InnerVal: &sconfig.KeyDataFile{
								Path: pathToTestData("system_keys/other-system"),
							},
						},
					},
				},
			},
			AdminApi: sconfig.ServiceAdminApi{
				ServiceHttp: sconfig.ServiceHttp{
					PortVal: &sconfig.IntegerValue{&sconfig.IntegerValueDirect{Value: 8080}},
				},
			},
		})
		serv2 := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), nil, nil, nil, test_utils.NewTestLogger())

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

	t.Run("self-signed actor", func(t *testing.T) {
		// Set up actor key config
		bobdoleKey := &sconfig.Key{
			InnerVal: &sconfig.KeyPublicPrivate{
				PublicKey: &sconfig.KeyData{
					InnerVal: &sconfig.KeyDataFile{
						Path: pathToTestData("admin_user_keys/bobdole.pub"),
					},
				},
			},
		}

		cfg := config.FromRoot(&sconfig.Root{
			SystemAuth: sconfig.SystemAuth{
				Actors: &sconfig.ConfiguredActors{
					InnerVal: sconfig.ConfiguredActorsList{
						&sconfig.ConfiguredActor{
							ExternalId: "bobdole",
							Key:        bobdoleKey,
						},
					},
				},
				JwtTokenDurationVal: 12 * time.Hour,
				JwtIssuerVal:        "example",
				JwtSigningKey: &sconfig.Key{
					&sconfig.KeyPublicPrivate{
						PublicKey: &sconfig.KeyData{
							InnerVal: &sconfig.KeyDataFile{
								Path: pathToTestData("system_keys/other-system.pub"),
							},
						},
						PrivateKey: &sconfig.KeyData{
							InnerVal: &sconfig.KeyDataFile{
								Path: pathToTestData("system_keys/other-system"),
							},
						},
					},
				},
			},
			AdminApi: sconfig.ServiceAdminApi{
				ServiceHttp: sconfig.ServiceHttp{
					PortVal: &sconfig.IntegerValue{&sconfig.IntegerValueDirect{Value: 8080}},
				},
			},
		})

		// Set up database and encrypt service for self-signed tests
		var testDb database.DB
		var enc encrypt.E
		cfg, testDb = database.MustApplyBlankTestDbConfig(t, cfg)
		cfg, enc = encrypt.NewTestEncryptService(cfg, testDb)

		ctx := context.Background()

		// Marshal and encrypt the actor key for storage in database
		keyJson, err := json.Marshal(bobdoleKey)
		require.NoError(t, err)
		encryptedKey, err := enc.EncryptStringGlobal(ctx, string(keyJson))
		require.NoError(t, err)

		// Create the actor in the database
		err = testDb.CreateActor(ctx, &database.Actor{
			Id:           uuid.New(),
			Namespace:    "root",
			ExternalId:   "bobdole",
			EncryptedKey: &encryptedKey,
		})
		require.NoError(t, err)

		actorSrv := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), testDb, nil, enc, test_utils.NewTestLogger())

		t.Run("valid", func(t *testing.T) {
			token, err := jwt2.NewJwtTokenBuilder().
				WithActorExternalId("bobdole").
				WithPrivateKeyPath(pathToTestData("admin_user_keys/bobdole")).
				WithSelfSigned().
				WithAudience(string(sconfig.ServiceIdAdminApi)).
				TokenCtx(testContext)
			require.NoError(t, err)

			claims, err := actorSrv.Parse(testContext, token)
			require.NoError(t, err)
			require.Equal(t, "bobdole", claims.Subject)
		})

		t.Run("unknown actor", func(t *testing.T) {
			token, err := jwt2.NewJwtTokenBuilder().
				WithActorExternalId("billclinton").
				WithPrivateKeyPath(pathToTestData("admin_user_keys/billclinton")).
				WithSelfSigned().
				TokenCtx(testContext)
			require.NoError(t, err)

			_, err = actorSrv.Parse(testContext, token)
			require.Error(t, err)
		})

		t.Run("wrong key for actor", func(t *testing.T) {
			token, err := jwt2.NewJwtTokenBuilder().
				WithActorExternalId("bobdole").
				WithPrivateKeyPath(pathToTestData("admin_user_keys/billclinton")).
				WithSelfSigned().
				TokenCtx(testContext)
			require.NoError(t, err)

			_, err = actorSrv.Parse(testContext, token)
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
		cfg, db = database.MustApplyBlankTestDbConfig(t, cfg)
		a = NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), db, nil, nil, test_utils.NewTestLogger())
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
				require.Equal(t, testClaims().Actor.ExternalId, ra.MustGetActor().ExternalId)

				actor, err := db.GetActorByExternalId(testContext, "root", testClaims().Actor.ExternalId)
				require.NoError(t, err)
				require.Equal(t, testClaims().Actor.ExternalId, actor.ExternalId)
			})

			t.Run("actor loaded from database", func(t *testing.T) {
				setup(t)

				dbActorId := uuid.New()
				dbActor := &database.Actor{
					Id:         dbActorId,
					Namespace:  "root",
					ExternalId: testClaims().Actor.ExternalId,
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
				require.Equal(t, testClaims().Actor.ExternalId, ra.MustGetActor().ExternalId)
			})

			t.Run("actor permissions updated in database", func(t *testing.T) {
				setup(t)

				dbActorId := uuid.New()
				externalId := "perm-test-actor"
				oldPerms := database.Permissions{
					{
						Namespace: "root",
						Resources: []string{"connections"},
						Verbs:     []string{"read"},
					},
				}
				dbActor := &database.Actor{
					Id:          dbActorId,
					Namespace:   "root",
					ExternalId:  externalId,
					Permissions: oldPerms,
				}
				require.NoError(t, db.CreateActor(testContext, dbActor))

				// Verify old permissions are in DB
				retrieved, err := db.GetActorByExternalId(testContext, "root", externalId)
				require.NoError(t, err)
				require.Equal(t, oldPerms, retrieved.Permissions)

				// Create JWT with new permissions
				newPerms := []aschema.Permission{
					{
						Namespace: "root",
						Resources: []string{"connections", "connectors"},
						Verbs:     []string{"read", "create", "delete"},
					},
				}
				claims := &jwt2.AuthProxyClaims{
					RegisteredClaims: jwt.RegisteredClaims{
						ID:        "random id",
						Subject:   externalId,
						Issuer:    "remark42",
						Audience:  []string{string(sconfig.ServiceIdAdminApi)},
						ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
						NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
						IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
					},
					Actor: &core.Actor{
						ExternalId:  externalId,
						Namespace:   "root",
						Permissions: newPerms,
					},
				}

				tok, err := a.Token(testContext, claims)
				require.NoError(t, err)

				req := httptest.NewRequest("GET", "/", nil)
				req.Header.Add(JwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
				w := httptest.NewRecorder()
				ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
				require.NoError(t, err)
				require.True(t, ra.IsAuthenticated())
				require.Equal(t, externalId, ra.MustGetActor().ExternalId)

				// Verify the returned auth has new permissions
				require.Equal(t, newPerms, ra.MustGetActor().Permissions)

				// Verify the database was updated with new permissions
				retrieved, err = db.GetActorByExternalId(testContext, "root", externalId)
				require.NoError(t, err)
				require.Equal(t, dbActorId, retrieved.Id, "should preserve original database ID")
				require.Equal(t, database.Permissions(newPerms), retrieved.Permissions, "permissions should be updated in database")
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

			actor, err := db.GetActorByExternalId(testContext, "root", testClaims().Actor.ExternalId)
			require.ErrorIs(t, err, database.ErrNotFound)
			require.Nil(t, actor)
		})

		t.Run("bad token", func(t *testing.T) {
			setup(t)

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Add(JwtHeaderKey, "Bearer: bad bad token")
			w := httptest.NewRecorder()
			_, err := raw.establishAuthFromRequest(testContext, true, req, w)
			require.NotNil(t, err)

			actor, err := db.GetActorByExternalId(testContext, "root", testClaims().Actor.ExternalId)
			require.ErrorIs(t, err, database.ErrNotFound)
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

			require.Equal(t, ra.MustGetActor().Id, ra.MustGetActor().Id)
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

func TestAuth_ActorPermissionsSync(t *testing.T) {
	// Test that actor permissions are synced from config when the actor already exists in DB
	configPerms := []aschema.Permission{
		{
			Namespace: "root",
			Resources: []string{"connections", "connectors"},
			Verbs:     []string{"read", "create", "delete"},
		},
	}

	actorConfig := sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			JwtTokenDurationVal: 12 * time.Hour,
			JwtIssuerVal:        "example",
			JwtSigningKey: &sconfig.Key{
				InnerVal: &sconfig.KeyPublicPrivate{
					PublicKey: &sconfig.KeyData{
						InnerVal: &sconfig.KeyDataFile{
							Path: pathToTestData("system_keys/system.pub"),
						},
					},
					PrivateKey: &sconfig.KeyData{
						InnerVal: &sconfig.KeyDataFile{
							Path: pathToTestData("system_keys/system"),
						},
					},
				},
			},
			Actors: &sconfig.ConfiguredActors{
				InnerVal: sconfig.ConfiguredActorsList{
					&sconfig.ConfiguredActor{
						ExternalId:  "aid1",
						Permissions: configPerms,
						Key: &sconfig.Key{
							InnerVal: &sconfig.KeyPublicPrivate{
								PublicKey: &sconfig.KeyData{
									InnerVal: &sconfig.KeyDataFile{
										Path: pathToTestData("system_keys/system.pub"),
									},
								},
								PrivateKey: &sconfig.KeyData{
									InnerVal: &sconfig.KeyDataFile{
										Path: pathToTestData("system_keys/system"),
									},
								},
							},
						},
					},
				},
			},
			GlobalAESKey: &sconfig.KeyData{
				InnerVal: &sconfig.KeyDataBase64Val{
					Base64: "tOqE5HtiujnwB7pXt6lQLH8/gCh6TmMq9uSLFtJxZtU=",
				},
			},
		},
		AdminApi: sconfig.ServiceAdminApi{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: &sconfig.IntegerValue{&sconfig.IntegerValueDirect{Value: 8080}},
			},
		},
	}

	var a A
	var raw *service
	var db database.DB

	setup := func(t *testing.T) {
		cfg := config.FromRoot(&actorConfig)
		cfg, db = database.MustApplyBlankTestDbConfig(t, cfg)
		a = NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), db, nil, nil, test_utils.NewTestLogger())
		raw = a.(*service)
	}

	t.Run("actor auth uses permissions from database", func(t *testing.T) {
		setup(t)

		actorExternalId := "aid1"
		dbPerms := database.Permissions{
			{
				Namespace: "root",
				Resources: []string{"connections"},
				Verbs:     []string{"read"},
			},
		}

		// Create actor in database with permissions (simulating actor sync)
		dbActorId := uuid.New()
		dbActor := &database.Actor{
			Id:          dbActorId,
			Namespace:   "root",
			ExternalId:  actorExternalId,
			Permissions: dbPerms,
		}
		require.NoError(t, db.CreateActor(testContext, dbActor))

		// Create JWT for actor WITHOUT Actor field (should use existing actor from DB)
		claims := &jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Subject:   actorExternalId,
				Issuer:    "remark42",
				Audience:  []string{string(sconfig.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{Time: time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{Time: time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{Time: apctx.GetClock(testContext).Now()},
			},
			Actor: nil, // No actor - should use existing actor from database
		}

		tok, err := a.Token(testContext, claims)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Add(JwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
		w := httptest.NewRecorder()
		ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
		require.NoError(t, err)
		require.True(t, ra.IsAuthenticated())
		require.Equal(t, actorExternalId, ra.MustGetActor().ExternalId)

		// Verify the returned auth has permissions from database (not from config)
		// Cast to []aschema.Permission for comparison since database.Permissions is a type alias
		require.Equal(t, []aschema.Permission(dbPerms), ra.MustGetActor().Permissions)
	})

	t.Run("actor auth fails if not in database", func(t *testing.T) {
		setup(t)

		actorExternalId := "aid1"

		// Create JWT for actor that doesn't exist in database
		claims := &jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ID:        "random id",
				Subject:   actorExternalId,
				Issuer:    "remark42",
				Audience:  []string{string(sconfig.ServiceIdAdminApi)},
				ExpiresAt: &jwt.NumericDate{Time: time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
				NotBefore: &jwt.NumericDate{Time: time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
				IssuedAt:  &jwt.NumericDate{Time: apctx.GetClock(testContext).Now()},
			},
			Actor: nil, // No actor
		}

		tok, err := a.Token(testContext, claims)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Add(JwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
		w := httptest.NewRecorder()
		ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
		require.Error(t, err) // Should fail because actor not in database
		require.False(t, ra.IsAuthenticated())
	})
}

func TestAuth_ActorCacheReducesDbCalls(t *testing.T) {
	// When claims.Actor is nil, the auth flow calls GetActorByExternalId in keyForToken
	// (during JWT verification) and again in establishAuthFromRequest (to build RequestAuth).
	// With the actor cache, the second call should be served from cache, so the DB should
	// only be queried once.
	t.Parallel()

	cfg := config.FromRoot(&testConfigPublicPrivateKey)
	ctrl := gomock.NewController(t)
	mockDb := mock.NewMockDB(ctrl)

	actorId := uuid.New()
	actor := &database.Actor{
		Id:         actorId,
		Namespace:  "root",
		ExternalId: "id1",
	}

	// The critical assertion: GetActorByExternalId should only be called once,
	// not twice, because the second lookup hits the cache.
	mockDb.
		EXPECT().
		GetActorByExternalId(gomock.Any(), "root", "id1").
		Return(actor, nil).
		Times(1)

	authService := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), mockDb, nil, nil, test_utils.NewTestLogger())
	raw := authService.(*service)

	claims := *testClaims()
	claims.Actor = nil // Force actor lookup from database

	tok, err := authService.Token(testContext, &claims)
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add(JwtHeaderKey, fmt.Sprintf("Bearer %s", tok))
	w := httptest.NewRecorder()
	ra, err := raw.establishAuthFromRequest(testContext, true, req, w)
	require.NoError(t, err)
	require.True(t, ra.IsAuthenticated())
	require.Equal(t, "id1", ra.MustGetActor().ExternalId)
	require.Equal(t, actorId, ra.MustGetActor().Id)
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
		cfg, auth, authUtil := TestAuthService(t, sconfig.ServiceIdAdminApi, cfg)
		r := gin.Default()
		r.GET("/", auth.Required(), func(c *gin.Context) {
			ra := MustGetAuthFromGinContext(c)
			c.String(200, ra.MustGetActor().ExternalId)
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
		require.Equal(t, c.Actor.ExternalId, w.Body.String())
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
		require.Equal(t, c.Actor.ExternalId, w.Body.String())

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
			Audience:  []string{string(sconfig.ServiceIdAdminApi)},
			ExpiresAt: &jwt.NumericDate{time.Date(2058, 5, 21, 7, 30, 22, 0, time.UTC)},
			NotBefore: &jwt.NumericDate{time.Date(2018, 5, 21, 6, 30, 22, 0, time.UTC)},
			IssuedAt:  &jwt.NumericDate{apctx.GetClock(testContext).Now()},
		},

		Namespace: "root",
		Actor: &core.Actor{
			ExternalId: "id1",
			Namespace:  "root",
		},
	}
}

var testConfigPublicPrivateKey = sconfig.Root{
	SystemAuth: sconfig.SystemAuth{
		JwtTokenDurationVal: 12 * time.Hour,
		JwtIssuerVal:        "example",
		JwtSigningKey: &sconfig.Key{
			InnerVal: &sconfig.KeyPublicPrivate{
				PublicKey: &sconfig.KeyData{
					InnerVal: &sconfig.KeyDataFile{
						Path: pathToTestData("system_keys/system.pub"),
					},
				},
				PrivateKey: &sconfig.KeyData{
					InnerVal: &sconfig.KeyDataFile{
						Path: pathToTestData("system_keys/system"),
					},
				},
			},
		},
		Actors: &sconfig.ConfiguredActors{
			InnerVal: sconfig.ConfiguredActorsList{
				&sconfig.ConfiguredActor{
					ExternalId: "aid1",
					Key: &sconfig.Key{
						InnerVal: &sconfig.KeyPublicPrivate{
							PublicKey: &sconfig.KeyData{
								InnerVal: &sconfig.KeyDataFile{
									Path: pathToTestData("system_keys/system.pub"),
								},
							},
							PrivateKey: &sconfig.KeyData{
								InnerVal: &sconfig.KeyDataFile{
									Path: pathToTestData("system_keys/system"),
								},
							},
						},
					},
				},
			},
		},
		GlobalAESKey: &sconfig.KeyData{
			InnerVal: &sconfig.KeyDataBase64Val{
				Base64: "tOqE5HtiujnwB7pXt6lQLH8/gCh6TmMq9uSLFtJxZtU=",
			},
		},
	},
	AdminApi: sconfig.ServiceAdminApi{
		ServiceHttp: sconfig.ServiceHttp{
			PortVal: &sconfig.IntegerValue{&sconfig.IntegerValueDirect{Value: 8080}},
		},
	},
}

var testConfigSecretKey = sconfig.Root{
	SystemAuth: sconfig.SystemAuth{
		JwtTokenDurationVal: 12 * time.Hour,
		JwtIssuerVal:        "example",
		JwtSigningKey: &sconfig.Key{
			InnerVal: &sconfig.KeyShared{
				SharedKey: &sconfig.KeyData{
					InnerVal: &sconfig.KeyDataBase64Val{
						Base64: "+xKbTv+pdvWK+4ucIsUcAHqzEhelLWuud80+fy1pQzc=",
					},
				},
			},
		},
		GlobalAESKey: &sconfig.KeyData{
			InnerVal: &sconfig.KeyDataBase64Val{
				Base64: "tOqE5HtiujnwB7pXt6lQLH8/gCh6TmMq9uSLFtJxZtU=",
			},
		},
	},
	AdminApi: sconfig.ServiceAdminApi{
		ServiceHttp: sconfig.ServiceHttp{
			PortVal: &sconfig.IntegerValue{&sconfig.IntegerValueDirect{Value: 8080}},
		},
	},
}
