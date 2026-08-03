package config

import (
	"context"
	"net/url"

	"github.com/rmorlok/authproxy/internal/schema/common"
)

type HostApplication struct {
	// InitiateSessionUrl is the URL that will be redirected to in order to establish a session for an actor. This
	// happens if the marketplace portal is accessed without coming from a pre-authorized context. This URL should
	// take a `returnToUrl` query parameter where the actor should be redirected to following successful authentication.
	// When redirecting to `returnToUrl`, the host application should append an `authToken` query param with a signed
	// JWT for authenticating the user. This JWT should use a nonce and expiration to protect against session
	// hijacking
	InitiateSessionUrl *StringValue `json:"initiateSessionUrl" yaml:"initiateSessionUrl"`
}

func (ha *HostApplication) Validate(vc *common.ValidationContext) error {
	if ha == nil {
		return vc.NewError("hostApplication must be specified")
	}

	if ha.InitiateSessionUrl == nil || !ha.InitiateSessionUrl.HasValue(context.Background()) {
		return vc.NewError("initiateSessionUrl must be specified")
	}

	return nil
}

func (ha *HostApplication) GetInitiateSessionUrl(returnTo string) string {
	raw := ""
	if ha.InitiateSessionUrl != nil {
		raw, _ = ha.InitiateSessionUrl.GetValue(context.Background())
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	q := u.Query()
	q.Set("returnToUrl", returnTo)
	u.RawQuery = q.Encode()

	return u.String()
}
