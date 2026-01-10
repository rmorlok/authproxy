package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/schema/config"
)

const (
	sessionCookieName = "SESSION-ID"
	xsrfHeaderKey     = "X-XSRF-TOKEN"
)

type Encrypt interface {
	EncryptGlobal(ctx context.Context, data []byte) ([]byte, error)
	DecryptGlobal(ctx context.Context, data []byte) ([]byte, error)
}

// session is the object stored in redis to track the session
type session struct {
	Id              uuid.UUID `json:"id"`
	ActorId         uuid.UUID `json:"actor_id"`
	ValidXsrfValues []string  `json:"-"` // Serialized separately in a different key
	ExpiresAt       time.Time `json:"expires_at"`
}

func (s *session) MarshalBinary() ([]byte, error) {
	return json.Marshal(s)
}

func (s *session) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, s)
}

func (s *session) IsValid() bool {
	return s.Id != uuid.Nil && s.ActorId != uuid.Nil && !s.ExpiresAt.IsZero()
}

func (s *session) IsExpired(ctx context.Context) bool {
	return s.ExpiresAt.Before(apctx.GetClock(ctx).Now())
}

func (s *session) GetSessionId() sessionId {
	return sessionId{
		Id:      s.Id,
		ActorId: s.ActorId,
	}
}

// sessionId is the data used to compute a secure id that is sent as the session cookie. This data overlaps with
// data in the session itself so that sessions can't be hijacked by changing data in redis
type sessionId struct {
	Id      uuid.UUID `json:"id"`
	ActorId uuid.UUID `json:"actor_id"`
}

func (s *sessionId) GetSessionCookieId(e Encrypt) (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal session id")
	}

	encrypted, err := e.EncryptGlobal(context.Background(), data)
	if err != nil {
		return "", errors.Wrap(err, "failed to encrypt session id")
	}

	return base64.RawURLEncoding.EncodeToString(encrypted), nil
}

func fromSessionCookieId(val string, e Encrypt) (sessionId, error) {
	data, err := base64.RawURLEncoding.DecodeString(val)
	if err != nil {
		return sessionId{}, errors.Wrap(err, "failed to decode session id")
	}

	decrypted, err := e.DecryptGlobal(context.Background(), data)
	if err != nil {
		return sessionId{}, errors.Wrap(err, "failed to decrypt session id")
	}

	var sid sessionId
	err = json.Unmarshal(decrypted, &sid)
	if err != nil {
		return sessionId{}, errors.Wrap(err, "failed to unmarshal session id")
	}

	return sid, nil
}

// establishAuthFromSession loads auth from an existing session. It is used as part of the process of attempting
// to establish auth for a request. It takes an optional RequestAuth which would have come from any JWT present on
// the request previously.
func (s *service) establishAuthFromSession(
	ctx context.Context,
	requireSessionXsrf bool,
	r *http.Request,
	w http.ResponseWriter,
	fromJwt *core.RequestAuth,
) (*core.RequestAuth, error) {
	if !s.service.SupportsSession() {
		// This service doesn't support sessions, so do nothing
		return fromJwt, nil
	}

	// Read the session ID from the cookie
	sessionCookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		// If the session cookie is not found, return the original JWT auth
		if errors.Is(err, http.ErrNoCookie) {
			return fromJwt, nil
		}
		// If there is another error, return it
		return fromJwt, errors.Wrap(err, "failed to read session cookie")
	}

	// Parse the session cookie ID from the cookie's value
	sessionCookieId, err := fromSessionCookieId(sessionCookie.Value, s.encrypt)
	if err != nil || sessionCookieId.Id == uuid.Nil {
		return fromJwt, errors.Wrap(err, "invalid session ID in cookie")
	}

	sess, err := s.tryReadSessionFromRedis(ctx, sessionCookieId.Id)
	if err != nil {
		return fromJwt, errors.Wrap(err, "failed to read session from redis")
	}

	if sess == nil {
		// No session found. Whatever the JWT has stands.
		return fromJwt, nil
	}

	// Sanity check
	if sess.Id != sessionCookieId.Id {
		return core.NewUnauthenticatedRequestAuth(), errors.Errorf("session id mismatch: %s != %s", sess.Id.String(), sessionCookieId.Id.String())
	}

	// Sanity check to avoid session hijacking in redis
	if sess.ActorId != sessionCookieId.ActorId {
		return core.NewUnauthenticatedRequestAuth(), errors.Errorf("session actor mismatch: %s != %s", sess.ActorId.String(), sessionCookieId.ActorId.String())
	}

	if fromJwt.IsAuthenticated() {
		// Sanity check that this session is for the same actor
		if sess.ActorId != fromJwt.GetActor().Id {
			// The two do not agree. Cancel the previous session as this is a new user. Service will need to
			// re-establish session if it wants it.
			err = s.deleteSessionFromRedis(ctx, sessionCookieId.Id)
			if err != nil {
				return core.NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to delete session from redis")
			}

			return fromJwt, nil
		}

		// The session agrees with the user. The user is "doubly" authenticated.
		fromJwt.SetSessionId(&sess.Id)
		return fromJwt, nil
	} else {
		// User was not authenticated by JWT but does have a valid session. In order to authenticate this request, we
		// must validate if the XSRF token is present if this request is for a non-GET request.

		// Check for XSRF token in the header for non-GET requests
		if requireSessionXsrf && r.Method != http.MethodGet {
			xsrfTokenHeader := r.Header.Get(xsrfHeaderKey)
			if xsrfTokenHeader == "" {
				return core.NewUnauthenticatedRequestAuth(), api_common.
					NewHttpStatusErrorBuilder().
					WithStatusForbidden().
					WithResponseMsg("missing XSRF token").
					Build()
			}

			isValidXsrf := false

			// Validate the XSRF token against valid CSRF values in the session
			for _, validToken := range sess.ValidXsrfValues {
				if xsrfTokenHeader == validToken {
					isValidXsrf = true
					break
				}
			}

			if !isValidXsrf {
				return core.NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusForbidden().
					WithResponseMsg("invalid XSRF token").
					Build()
			}
		}
	}

	actor, err := s.db.GetActor(ctx, sess.ActorId)
	if err != nil {
		return core.NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to get actor from database")
	}

	err = s.extendSession(ctx, sess, w)
	if err != nil {
		return core.NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to extend session")
	}

	return core.NewAuthenticatedRequestAuthWithSession(actor, &sess.Id), nil
}

// EstablishSession is used to start a new session explicitly from a service that is using auth. Generally this
// will be used to session a user after that request has already been authenticated using a JWT. This method does
// check for existing sessions and either extends them or cancels them if the auth is inconsistent.
func (s *service) EstablishSession(ctx context.Context, w http.ResponseWriter, ra *core.RequestAuth) error {
	if !ra.IsAuthenticated() {
		return errors.New("request is not authenticated")
	}

	if !s.service.SupportsSession() {
		return errors.Errorf("server %s does not support session", s.service.GetId())
	}

	sessionService, ok := s.service.(config.HttpServiceWithSession)
	if !ok {
		return errors.Errorf("server %s is misconfigured; it inidicates it supports session but does not implement appropriate interfaces", s.service.GetId())
	}

	var err error
	var sess *session

	if ra.IsSession() {
		sessionId := *ra.GetSessionId()
		sess, err = s.tryReadSessionFromRedis(ctx, sessionId)
		if err != nil {
			return errors.Wrap(err, "failed to read session from redis")
		}

		if sess != nil {
			// Sanity check that this session is for the same actor
			if sess.ActorId != ra.GetActor().Id {
				err = s.deleteSessionFromRedis(ctx, sessionId)
				if err != nil {
					return errors.Wrap(err, "failed to delete session from redis")
				}
				sess = nil
			}
		}
	}

	if sess == nil {
		sess = &session{
			Id:        uuid.New(),
			ActorId:   ra.GetActor().Id,
			ExpiresAt: apctx.GetClock(ctx).Now().Add(sessionService.SessionTimeout()),
		}
	}

	err = s.extendSession(ctx, sess, w)
	if err != nil {
		return errors.Wrap(err, "failed to establish session")
	}

	if s.setSessionCookie(ctx, w, *sess) != nil {
		return errors.Wrap(err, "failed to set session cookie")
	}

	ra.SetSessionId(&sess.Id)

	return nil
}

func (s *service) extendSession(ctx context.Context, sess *session, w http.ResponseWriter) error {
	sessionService := s.service.(config.HttpServiceWithSession)
	validXsrfValuesLimit := int64(sessionService.XsrfRequestQueueDepth())

	// Generate a new UUID to push onto the list.
	newXsrfValue := uuid.New()

	pipe := s.r.Pipeline()
	pipe.Set(ctx, getRedisSessionJsonKey(sess.Id), sess, sess.ExpiresAt.Sub(apctx.GetClock(ctx).Now()))
	pipe.LPush(ctx, getRedisXsrfKey(sess.Id), newXsrfValue.String())
	pipe.LTrim(ctx, getRedisXsrfKey(sess.Id), 0, validXsrfValuesLimit)
	pipe.Expire(ctx, getRedisXsrfKey(sess.Id), sess.ExpiresAt.Sub(apctx.GetClock(ctx).Now()))

	// Write the session to Redis
	if _, err := pipe.Exec(ctx); err != nil {
		return errors.Wrap(err, "failed to write session to redis")
	}

	// Pass down the new XSRF token
	w.Header().Set(xsrfHeaderKey, newXsrfValue.String())

	return nil
}

func (s *service) setSessionCookie(ctx context.Context, w http.ResponseWriter, sess session) error {
	sessionService := s.service.(config.HttpServiceWithSession)
	cookieExpiration := 0 // session cookie
	if sessionService.SessionTimeout() != 0 {
		cookieExpiration = int(sess.ExpiresAt.Sub(apctx.GetClock(ctx).Now()).Seconds())
	}

	sessId := sess.GetSessionId()
	val, err := sessId.GetSessionCookieId(s.encrypt)
	if err != nil {
		return errors.Wrap(err, "failed to get session cookie id")
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    val,
		HttpOnly: true,
		Path:     "/",
		Domain:   sessionService.CookieDomain(),
		MaxAge:   cookieExpiration,
		Secure:   s.service.IsHttps(),
		SameSite: sessionService.CookieSameSite(),
	})

	return nil
}

// EndSession terminates a session that is in progress by clearing the session information from redis and clearing
// session id cookies on the response.
func (s *service) EndSession(ctx context.Context, w http.ResponseWriter, ra *core.RequestAuth) error {
	if ra.IsSession() {
		err := s.deleteSessionFromRedis(ctx, *ra.GetSessionId())
		if err != nil {
			return errors.Wrap(err, "failed to delete session from redis")
		}
	}

	sessionService := s.service.(config.HttpServiceWithSession)
	sessionCookie := http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		HttpOnly: false,
		Path:     "/",
		Domain:   sessionService.CookieDomain(),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		Secure:   s.service.IsHttps(),
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, &sessionCookie)

	return nil
}

func getRedisSessionKeys(sessionId uuid.UUID) []string {
	return []string{
		getRedisSessionJsonKey(sessionId),
		getRedisXsrfKey(sessionId),
	}
}

func getRedisSessionJsonKey(sessionId uuid.UUID) string {
	return "session:" + sessionId.String() + ":json"
}

func getRedisXsrfKey(sessionId uuid.UUID) string {
	return "session:" + sessionId.String() + ":xsrf"
}

func (s *service) deleteSessionFromRedis(ctx context.Context, sessionId uuid.UUID) error {
	if err := s.r.Del(ctx, getRedisSessionKeys(sessionId)...).Err(); err != nil {
		if err == redis.Nil {
			// Key does not exist, this is not an error
			return nil
		}
		return errors.Wrap(err, "failed to delete session key from redis")
	}

	return nil
}

// tryReadSessionFromRedis attempts to get session from Redis. Returns nil if the session does not exist, or is expired.
func (s *service) tryReadSessionFromRedis(ctx context.Context, sessionId uuid.UUID) (*session, error) {
	pipe := s.r.Pipeline()

	jsonData := pipe.Get(ctx, getRedisSessionJsonKey(sessionId))
	xsrfData := pipe.LRange(ctx, getRedisXsrfKey(sessionId), 0, -1)

	_, err := pipe.Exec(ctx)

	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Not an error, just no session in redis
			return nil, nil
		}

		return nil, errors.Wrapf(err, "failed to get session from redis for id %s", sessionId.String())
	}

	var sess session
	if err := jsonData.Scan(&sess); err != nil {
		return nil, errors.Wrap(err, "failed to parse session from redis value")
	}

	if !sess.IsValid() {
		return nil, errors.Errorf("session %s is invalid", sessionId.String())
	}

	if sess.IsExpired(ctx) {
		// this is not an error, treat it like there is no session
		return nil, nil
	}

	if sess.ValidXsrfValues, err = xsrfData.Result(); err != nil {
		return nil, errors.Wrap(err, "failed to get XSRF values from redis")
	}

	return &sess, nil
}
