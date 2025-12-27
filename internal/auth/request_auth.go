package auth

import (
	"context"
	context2 "context"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
)

const authContextKey = "auth"

// GetAuthFromContext gets the auth from context. If no auth is in context, it returns an unauthenticated auth.
func GetAuthFromContext(ctx context2.Context) *RequestAuth {
	if a, ok := ctx.Value(authContextKey).(*RequestAuth); ok {
		return a
	}

	return NewUnauthenticatedRequestAuth()
}

type RequestAuth struct {
	sessionId *uuid.UUID
	actor     *database.Actor
}

func (ra *RequestAuth) IsAuthenticated() bool {
	return ra.actor != nil
}

func (ra *RequestAuth) IsSession() bool {
	return ra.IsAuthenticated() && ra.sessionId != nil
}

func (ra *RequestAuth) getSessionId() *uuid.UUID {
	return ra.sessionId
}

func (ra *RequestAuth) setSessionId(sessionId *uuid.UUID) {
	ra.sessionId = sessionId
}

func (ra *RequestAuth) GetActor() *database.Actor {
	return ra.actor
}

func (ra *RequestAuth) MustGetActor() database.Actor {
	if ra.actor == nil {
		panic("request was expected to be authenticated, but was not")
	}

	return *ra.actor
}

func (a *RequestAuth) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey, a)
}

func NewUnauthenticatedRequestAuth() *RequestAuth {
	return &RequestAuth{}
}

func NewAuthenticatedRequestAuth(a *database.Actor) *RequestAuth {
	return &RequestAuth{
		actor: a,
	}
}

func NewAuthenticatedRequestAuthWithSession(a *database.Actor, sess *uuid.UUID) *RequestAuth {
	return &RequestAuth{
		actor:     a,
		sessionId: sess,
	}
}
