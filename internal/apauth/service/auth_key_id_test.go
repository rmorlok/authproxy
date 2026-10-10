package service

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	jwt2 "github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// signedWithKeyId signs claims directly so tests control the kid header. A nil kid produces a legacy token.
func signedWithKeyId(t *testing.T, claims *jwt2.AuthProxyClaims, method jwt.SigningMethod, signingKey interface{}, kid *string) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	if kid != nil {
		token.Header[jwt2.KeyIdHeader] = *kid
	}
	s, err := token.SignedString(signingKey)
	require.NoError(t, err)
	return s
}

func loadTestPrivateKey(t *testing.T, path string) interface{} {
	t.Helper()
	data, err := os.ReadFile(pathToTestData(path))
	require.NoError(t, err)
	k, err := ssh.ParseRawPrivateKey(data)
	require.NoError(t, err)
	return k
}

func loadTestPublicKeyId(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(pathToTestData(path))
	require.NoError(t, err)
	pub, _, _, _, err := ssh.ParseAuthorizedKey(data)
	require.NoError(t, err)
	kid, err := jwt2.KeyIdForPublicKey(pub.(ssh.CryptoPublicKey).CryptoPublicKey())
	require.NoError(t, err)
	return kid
}

func TestAuth_ParseKeyId(t *testing.T) {
	const globalAESMockId = "auth-service-kid-global-aes"
	oldAESKey := []byte("old-global-aes-key-old-global-ae")
	newAESKey := []byte("new-global-aes-key-new-global-ae")
	sconfig.NewKeyDataMock(globalAESMockId)
	sconfig.KeyDataMockAddVersion(globalAESMockId, "aes", "v1", oldAESKey)
	sconfig.KeyDataMockAddVersion(globalAESMockId, "aes", "v2", newAESKey)

	bobdoleKey := &sconfig.Key{
		InnerVal: &sconfig.KeyPublicPrivate{
			PublicKey: &sconfig.KeyData{
				InnerVal: &sconfig.KeyDataFile{Path: pathToTestData("admin_user_keys/bobdole.pub")},
			},
		},
	}

	cfg := config.FromRoot(&sconfig.Root{
		SystemAuth: sconfig.SystemAuth{
			JwtTokenDurationVal: 12 * time.Hour,
			JwtIssuerVal:        "example",
			JwtSigningKey: &sconfig.Key{
				InnerVal: &sconfig.KeyPublicPrivate{
					PublicKey: &sconfig.KeyData{
						InnerVal: &sconfig.KeyDataFile{Path: pathToTestData("system_keys/other-system.pub")},
					},
					PrivateKey: &sconfig.KeyData{
						InnerVal: &sconfig.KeyDataFile{Path: pathToTestData("system_keys/other-system")},
					},
				},
			},
			GlobalAESKey: &sconfig.KeyData{InnerVal: &sconfig.KeyDataMock{MockID: globalAESMockId}},
		},
		AdminApi: sconfig.ServiceAdminApi{
			ServiceHttp: sconfig.ServiceHttp{
				PortVal: &sconfig.IntegerValue{InnerVal: &sconfig.IntegerValueDirect{Value: 8080}},
			},
		},
	})

	var testDb database.DB
	var enc encrypt.E
	cfg, testDb = database.MustApplyBlankTestDbConfig(t, cfg)
	cfg, enc = encrypt.NewTestEncryptService(cfg, testDb)

	ctx := context.Background()
	keyJson, err := json.Marshal(bobdoleKey)
	require.NoError(t, err)
	encryptedKey, err := enc.EncryptStringGlobal(ctx, string(keyJson))
	require.NoError(t, err)

	require.NoError(t, testDb.CreateActor(ctx, &database.Actor{
		Id:           apid.New(apid.PrefixActor),
		Namespace:    "root",
		ExternalId:   "bobdole",
		EncryptedKey: &encryptedKey,
	}))
	require.NoError(t, testDb.CreateActor(ctx, &database.Actor{
		Id:         apid.New(apid.PrefixActor),
		Namespace:  "root",
		ExternalId: "keyless",
	}))

	srv := NewService(cfg, cfg.MustGetService(sconfig.ServiceIdAdminApi).(sconfig.HttpService), testDb, nil, enc, test_utils.NewTestLogger())

	claimsFor := func(subject string) *jwt2.AuthProxyClaims {
		return &jwt2.AuthProxyClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:  subject,
				Audience: []string{string(sconfig.ServiceIdAdminApi)},
			},
		}
	}
	actorSignedClaims := func(subject string) *jwt2.AuthProxyClaims {
		c := claimsFor(subject)
		c.ActorSigned = true
		return c
	}
	systemSignedClaims := func(subject string) *jwt2.AuthProxyClaims {
		c := claimsFor(subject)
		c.SystemSigned = true
		return c
	}

	bobdolePrivate := loadTestPrivateKey(t, "admin_user_keys/bobdole")
	globalPrivate := loadTestPrivateKey(t, "system_keys/other-system")
	bobdoleKid := loadTestPublicKeyId(t, "admin_user_keys/bobdole.pub")
	globalKid := loadTestPublicKeyId(t, "system_keys/other-system.pub")
	oldAESKid := jwt2.KeyIdForSharedKey(oldAESKey)
	unknownKid := "ap1.AAAAAAAAAAAAAAAAAAAAAA"

	for _, tt := range []struct {
		name      string
		claims    *jwt2.AuthProxyClaims
		method    jwt.SigningMethod
		key       interface{}
		kid       *string
		expectErr bool
	}{
		// Actor-specific keys
		{name: "actor key with kid", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodEdDSA, key: bobdolePrivate, kid: &bobdoleKid},
		{name: "actor key legacy", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodEdDSA, key: bobdolePrivate},
		{name: "actor key without signing claim", claims: claimsFor("bobdole"), method: jwt.SigningMethodEdDSA, key: bobdolePrivate, kid: &bobdoleKid},
		{name: "actor key unknown kid", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodEdDSA, key: bobdolePrivate, kid: &unknownKid, expectErr: true},
		{name: "actor key with global kid", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodEdDSA, key: bobdolePrivate, kid: &globalKid, expectErr: true},

		// Global JWT signing key
		{name: "global key with kid for actor with key", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodRS256, key: globalPrivate, kid: &globalKid},
		{name: "global key legacy for actor with key", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodRS256, key: globalPrivate},
		{name: "global key with kid for keyless actor", claims: claimsFor("keyless"), method: jwt.SigningMethodRS256, key: globalPrivate, kid: &globalKid},
		{name: "global key legacy for keyless actor", claims: claimsFor("keyless"), method: jwt.SigningMethodRS256, key: globalPrivate},
		{name: "global key with actor kid", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodRS256, key: globalPrivate, kid: &bobdoleKid, expectErr: true},
		{name: "global key unknown kid", claims: claimsFor("keyless"), method: jwt.SigningMethodRS256, key: globalPrivate, kid: &unknownKid, expectErr: true},

		// System-signed tokens with GlobalAESKey rotation
		{name: "previous global AES version with kid", claims: systemSignedClaims("bobdole"), method: jwt.SigningMethodHS256, key: oldAESKey, kid: &oldAESKid},
		{name: "previous global AES version legacy", claims: systemSignedClaims("bobdole"), method: jwt.SigningMethodHS256, key: oldAESKey, expectErr: true},
		{name: "global AES with actor kid", claims: systemSignedClaims("bobdole"), method: jwt.SigningMethodHS256, key: newAESKey, kid: &bobdoleKid, expectErr: true},
		{name: "public key as HMAC secret", claims: actorSignedClaims("bobdole"), method: jwt.SigningMethodHS256, key: mustReadTestData(t, "admin_user_keys/bobdole.pub"), kid: &bobdoleKid, expectErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			token := signedWithKeyId(t, tt.claims, tt.method, tt.key, tt.kid)
			claims, err := srv.Parse(testContext, token)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.claims.Subject, claims.Subject)
		})
	}

	t.Run("service-minted tokens carry the current global AES kid", func(t *testing.T) {
		token, err := srv.Token(testContext, claimsFor("bobdole"))
		require.NoError(t, err)

		parsed, _, err := jwt.NewParser().ParseUnverified(token, &jwt2.AuthProxyClaims{})
		require.NoError(t, err)
		require.Equal(t, jwt2.KeyIdForSharedKey(newAESKey), parsed.Header[jwt2.KeyIdHeader])

		claims, err := srv.Parse(testContext, token)
		require.NoError(t, err)
		require.Equal(t, "bobdole", claims.Subject)
	})
}

func mustReadTestData(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(pathToTestData(path))
	require.NoError(t, err)
	return data
}
