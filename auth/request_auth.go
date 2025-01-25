package auth

import (
	"context"
	context2 "github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
)

const authContextKey = "auth"

// RequestAuth is the interface for objects that are returned for establishing auth methods.
type RequestAuth interface {
	IsAuthenticated() bool
	GetActor() *database.Actor
	MustGetActor() database.Actor
	ContextWith(ctx context.Context) context.Context
}

// GetAuthFromContext gets the auth from context. If no auth is in context, it returns an unauthenticated auth.
func GetAuthFromContext(ctx context2.Context) RequestAuth {
	if a, ok := ctx.Value(authContextKey).(RequestAuth); ok {
		return a
	}

	return NewUnauthenticatedRequestAuth()
}

type requestAuth struct {
	actor *database.Actor
}

func (ra *requestAuth) IsAuthenticated() bool {
	return ra.actor != nil
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
