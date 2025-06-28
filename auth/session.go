package auth

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/api_common"
	"github.com/rmorlok/authproxy/config"
	"net/http"
	"time"
)

const (
	sessionCookieName = "SESSION-ID"
	xsrfHeaderKey     = "X-XSRF-TOKEN"
)

type session struct {
	Id              uuid.UUID `json:"id"`
	ActorId         uuid.UUID `json:"actor_id"`
	ValidCsrfValues []string  `json:"valid_csrf_values"`
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

// establishAuthFromSession loads auth from an existing session. It is used as part of the process of attempting
// to establish auth for a request. It takes an optional claims which would have come from any JWT present on
// the request previously.
func (s *service) establishAuthFromSession(ctx context.Context, r *http.Request, w http.ResponseWriter, fromJwt RequestAuth) (RequestAuth, error) {
	if !s.service.SupportsSession() {
		// This service doesn't support sessions, so do nothing
		return fromJwt, nil
	}

	// Read the session ID from the cookie
	sessionCookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		// If the session cookie is not found, return the original JWT auth
		if err == http.ErrNoCookie {
			return fromJwt, nil
		}
		// If there is another error, return it
		return fromJwt, errors.Wrap(err, "failed to read session cookie")
	}

	// Parse the session ID from the cookie's value
	sessionId, err := uuid.Parse(sessionCookie.Value)
	if err != nil {
		return fromJwt, errors.Wrap(err, "invalid session ID in cookie")
	}

	sess, err := s.tryReadSessionFromRedis(ctx, sessionId)
	if err != nil {
		return fromJwt, errors.Wrap(err, "failed to read session from redis")
	}

	if sess == nil {
		// No session found. Whatever the JWT has stands.
		return fromJwt, nil
	}

	if fromJwt.IsAuthenticated() {
		// Sanity check that this session is for the same actor
		if sess.ActorId != fromJwt.GetActor().ID {
			// The two do not agree. Cancel the previous session as this is a new user. Service will need to
			// re-establish session if it wants it.
			err = s.deleteSessionFromRedis(ctx, sessionId)
			if err != nil {
				return NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to delete session from redis")
			}

			return fromJwt, nil
		}

		// The session agrees with the user. The user is "doubly" authenticated.
		fromJwt.setSessionId(&sess.Id)
		return fromJwt, nil
	} else {
		// User was not authenticated by JWT but does have a valid session. In order to authenticate this request, we
		// must validate if the XSRF token is present if this request is for a non-GET request.

		// Check for XSRF token in the header for non-GET requests
		if r.Method != http.MethodGet {
			xsrfTokenHeader := r.Header.Get(xsrfHeaderKey)
			if xsrfTokenHeader == "" {
				return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusForbidden().
					WithResponseMsg("missing XSRF token").
					Build()
			}

			isValidCsrf := false

			// Validate the XSRF token against valid CSRF values in the session
			for _, validToken := range sess.ValidCsrfValues {
				if xsrfTokenHeader == validToken {
					isValidCsrf = true
					break
				}
			}

			if !isValidCsrf {
				return NewUnauthenticatedRequestAuth(), api_common.NewHttpStatusErrorBuilder().
					WithStatusForbidden().
					WithResponseMsg("invalid XSRF token").
					Build()
			}
		}
	}

	actor, err := s.db.GetActor(ctx, sess.ActorId)
	if err != nil {
		return NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to get actor from database")
	}

	err = s.extendSession(ctx, sess, w)
	if err != nil {
		return NewUnauthenticatedRequestAuth(), errors.Wrap(err, "failed to extend session")
	}

	return &requestAuth{
		actor:     actor,
		sessionId: &sess.Id,
	}, nil
}

// EstablishSession is used to start a new session explicitly from a service that is using auth. Generally this
// will be used to session a user after that request has already been authenticated using a JWT. This method does
// check for existing sessions and either extends them or cancels them if the auth is inconsistent.
func (s *service) EstablishSession(ctx context.Context, w http.ResponseWriter, ra RequestAuth) error {
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
		sessionId := *ra.getSessionId()
		sess, err = s.tryReadSessionFromRedis(ctx, sessionId)
		if err != nil {
			return errors.Wrap(err, "failed to read session from redis")
		}

		if sess != nil {
			// Sanity check that this session is for the same actor
			if sess.ActorId != ra.GetActor().ID {
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
			ActorId:   ra.GetActor().ID,
			ExpiresAt: apctx.GetClock(ctx).Now().Add(sessionService.SessionTimeout()),
		}
	}

	err = s.extendSession(ctx, sess, w)
	if err != nil {
		return errors.Wrap(err, "failed to establish session")
	}

	s.setSessionCookie(ctx, w, *sess)
	ra.setSessionId(&sess.Id)

	return nil
}

func (s *service) extendSession(ctx context.Context, sess *session, w http.ResponseWriter) error {
	sessionService := s.service.(config.HttpServiceWithSession)
	validCsrfValuesLimit := sessionService.XsrfRequestQueueDepth()

	// Generate a new UUID to push onto the list.
	newCsrfValue := uuid.New()

	// Push the new value onto the session's list.
	sess.ValidCsrfValues = append(sess.ValidCsrfValues, newCsrfValue.String())

	// Truncate the list to the specified depth, treating it as a queue.
	if len(sess.ValidCsrfValues) > validCsrfValuesLimit {
		sess.ValidCsrfValues = sess.ValidCsrfValues[len(sess.ValidCsrfValues)-validCsrfValuesLimit:]
	}

	// Write the session to Redis
	key := getRedisSessionKey(sess.Id)
	if err := s.redis.Client().Set(ctx, key, sess, sess.ExpiresAt.Sub(apctx.GetClock(ctx).Now())).Err(); err != nil {
		return errors.Wrap(err, "failed to write session to redis")
	}

	// Pass down the new XSRF token
	w.Header().Set(xsrfHeaderKey, newCsrfValue.String())

	return nil
}

func (s *service) setSessionCookie(ctx context.Context, w http.ResponseWriter, sess session) {
	sessionService := s.service.(config.HttpServiceWithSession)
	cookieExpiration := 0 // session cookie
	if sessionService.SessionTimeout() != 0 {
		cookieExpiration = int(sess.ExpiresAt.Sub(apctx.GetClock(ctx).Now()).Seconds())
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.Id.String(),
		HttpOnly: true,
		Path:     "/",
		Domain:   sessionService.CookieDomain(),
		MaxAge:   cookieExpiration,
		Secure:   s.service.IsHttps(),
		SameSite: sessionService.CookieSameSite(),
	})
}

// EndSession terminates a session that is in progress by clearing the session information from redis and clearing
// session id cookies on the response.
func (s *service) EndSession(ctx context.Context, w http.ResponseWriter, ra RequestAuth) error {
	if ra.IsSession() {
		err := s.deleteSessionFromRedis(ctx, *ra.getSessionId())
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

func getRedisSessionKey(sessionId uuid.UUID) string {
	return "session:" + sessionId.String()
}

func (s *service) deleteSessionFromRedis(ctx context.Context, sessionId uuid.UUID) error {
	if err := s.redis.Client().Del(ctx, getRedisSessionKey(sessionId)).Err(); err != nil {
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
	result := s.redis.Client().Get(ctx, getRedisSessionKey(sessionId))

	if result.Err() != nil {
		if result.Err() == redis.Nil {
			// Not an error, just no session in redis
			return nil, nil
		}

		return nil, errors.Wrapf(result.Err(), "failed to get session from redis for id %s", sessionId.String())
	}

	var sess session
	if err := result.Scan(&sess); err != nil {
		return nil, errors.Wrap(err, "failed to parse session from redis value")
	}

	if !sess.IsValid() {
		return nil, errors.Errorf("session %s is invalid", sessionId.String())
	}

	if sess.IsExpired(ctx) {
		// this is not an error, treat it like there is no session
		return nil, nil
	}

	return &sess, nil
}
