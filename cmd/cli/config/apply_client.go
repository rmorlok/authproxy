package config

import (
	"time"

	"github.com/rmorlok/authproxy/internal/apply"
)

// ResolveApplyClient constructs the authenticated resource client for the
// forthcoming apply executor. Client dry-run must never call this method.
func (j *Resolver) ResolveApplyClient(timeout time.Duration) (*apply.Client, error) {
	options := apply.ClientOptions{Admin: j.admin, Timeout: timeout}
	var err error
	if j.admin {
		options.AdminAPIURL, err = j.ResolveAdminApiUrl()
	} else {
		options.APIURL, err = j.ResolveApiUrl()
	}
	if err != nil {
		return nil, err
	}
	options.Signer, err = j.ResolveSigner()
	if err != nil {
		return nil, err
	}
	return apply.NewClient(options)
}
