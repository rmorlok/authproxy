package httpf

import (
	"net/http"
)

type RoundTripperFactory interface {
	// NewRoundTripper returns a new http.RoundTripper. This method can return
	// nil to imply it does not want to participate in the request.
	NewRoundTripper(ri RequestInfo, transport http.RoundTripper) http.RoundTripper
}
