package auth

import (
	"context"
	context2 "context"
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/database"
)

const authContextKey = "auth"

// RequestAuth is the interface for objects that are returned for establishing auth methods.
type RequestAuth interface {
	// IsAuthenticated checks if the state of the request is authenticated.
	IsAuthenticated() bool

	// IsSession implies that the authentication came from a cookied session.
	IsSession() bool

	// GetActor returns a pointer to the Actor associated with the current request authentication state.
	GetActor() *database.Actor

	// MustGetActor returns the Actor associated with the request, panicking if the actor is nil or invalid.
	MustGetActor() database.Actor

	// ContextWith applies this request auth to a context (stores it in context)
	ContextWith(ctx context.Context) context.Context

	// getSessionId retrieves the session ID associated with the current request, or nil if no session exists.
	getSessionId() *uuid.UUID

	// setSessionId sets the session id on the request auth. This happens after a session is established for
	// an auth that began using a JWT.
	setSessionId(*uuid.UUID)
}

// GetAuthFromContext gets the auth from context. If no auth is in context, it returns an unauthenticated auth.
func GetAuthFromContext(ctx context2.Context) RequestAuth {
	if a, ok := ctx.Value(authContextKey).(RequestAuth); ok {
		return a
	}

	return NewUnauthenticatedRequestAuth()
}

type requestAuth struct {
	sessionId *uuid.UUID
	actor     *database.Actor
}

func (ra *requestAuth) IsAuthenticated() bool {
	return ra.actor != nil
}

func (ra *requestAuth) IsSession() bool {
	return ra.IsAuthenticated() && ra.sessionId != nil
}

func (ra *requestAuth) getSessionId() *uuid.UUID {
	return ra.sessionId
}

func (ra *requestAuth) setSessionId(sessionId *uuid.UUID) {
	ra.sessionId = sessionId
}

func (ra *requestAuth) GetActor() *database.Actor {
	return ra.actor
}

func (ra *requestAuth) MustGetActor() database.Actor {
	if ra.actor == nil {
		panic("request was expected to be authenticated, but was not")
	}

	return *ra.actor
}

func (a *requestAuth) ContextWith(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey, a)
}

var _ RequestAuth = &requestAuth{}

type unauthenticatedRequestAuth struct{}

func (ra *unauthenticatedRequestAuth) IsAuthenticated() bool {
	return false
}

func (ra *unauthenticatedRequestAuth) IsSession() bool {
	return false
}

func (ra *unauthenticatedRequestAuth) getSessionId() *uuid.UUID { return nil }

func (ra *unauthenticatedRequestAuth) setSessionId(sessionId *uuid.UUID) {}

func (ra *unauthenticatedRequestAuth) GetActor() *database.Actor {
	return nil
}

func (ra *unauthenticatedRequestAuth) MustGetActor() database.Actor {
	panic("request was expected to be authenticated, but was not")
}

func (ra *unauthenticatedRequestAuth) ContextWith(ctx context.Context) context.Context {
	// No-op because there is no auth
	return ctx
}

func NewUnauthenticatedRequestAuth() RequestAuth {
	return &unauthenticatedRequestAuth{}
}

var _ RequestAuth = &unauthenticatedRequestAuth{}
