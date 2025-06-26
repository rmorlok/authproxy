package config

import (
	"github.com/pkg/errors"
	"net/url"
)

type HostApplication struct {
	// InitiateSessionUrl is the URL that will be redirected to in order to establish a session for an actor. This
	// happens if the marketplace portal is accessed without coming from a pre-authorized context. This URL should
	// take a `redirect_url` query parameter where the actor should be redirected to following successful authentication.
	// When redirecting to `redirect_url`, the host application should append an `auth_token` query param with a signed
	// JWT for authenticating the user. This JWT should use a nonce and expiration to protect against session
	// hijacking
	InitiateSessionUrl string `json:"initiate_session_url" yaml:"initiate_session_url"`
}

func (ha *HostApplication) Validate() error {
	if ha == nil {
		return errors.New("host_application must be specified")
	}

	if ha.InitiateSessionUrl == "" {
		return errors.New("host_application.initiate_session_url must be specified")
	}

	return nil
}

func (ha *HostApplication) GetInitiateSessionUrl(returnTo string) string {
	u, err := url.Parse(ha.InitiateSessionUrl)
	if err != nil {
		return ha.InitiateSessionUrl
	}

	q := u.Query()
	q.Set("return_to", returnTo)
	u.RawQuery = q.Encode()

	return u.String()
}
