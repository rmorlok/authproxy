package core

import (
	"context"

	"github.com/google/uuid"
)

const authContextKey = "auth"

// GetAuthFromContext gets the auth from context. If no auth is in context, it returns an unauthenticated auth.
func GetAuthFromContext(ctx context.Context) *RequestAuth {
	if a, ok := ctx.Value(authContextKey).(*RequestAuth); ok {
		return a
	}

	return NewUnauthenticatedRequestAuth()
}

type RequestAuth struct {
	sessionId *uuid.UUID
	actor     *Actor
}

func (ra *RequestAuth) IsAuthenticated() bool {
	return ra.actor != nil
}

func (ra *RequestAuth) IsSession() bool {
	return ra.IsAuthenticated() && ra.sessionId != nil
}

func (ra *RequestAuth) GetSessionId() *uuid.UUID {
	return ra.sessionId
}

func (ra *RequestAuth) SetSessionId(sessionId *uuid.UUID) {
	ra.sessionId = sessionId
}

func (ra *RequestAuth) GetActor() *Actor {
	return ra.actor
}

func (ra *RequestAuth) MustGetActor() *Actor {
	if ra.actor == nil {
		panic("expected request to be authenticated")
	}

	return ra.actor
}

func (a *RequestAuth) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey, a)
}

func NewUnauthenticatedRequestAuth() *RequestAuth {
	return &RequestAuth{}
}

func NewAuthenticatedRequestAuth(a IActorData) *RequestAuth {
	return &RequestAuth{
		actor: CreateActor(a),
	}
}

func NewAuthenticatedRequestAuthWithSession(a IActorData, sess *uuid.UUID) *RequestAuth {
	return &RequestAuth{
		actor:     CreateActor(a),
		sessionId: sess,
	}
}
