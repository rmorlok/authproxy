package helpers

import (
	"net/http"

	"github.com/rmorlok/authproxy/internal/httpf"
)

// noopRoundTripperFactory is a no-op implementation of httpf.RoundTripperFactory.
type noopRoundTripperFactory struct{}

func (n *noopRoundTripperFactory) NewRoundTripper(_ httpf.RequestInfo, _ http.RoundTripper) http.RoundTripper {
	return nil
}

// NewNoopRoundTripperFactory returns a no-op RoundTripperFactory that never participates in requests.
func NewNoopRoundTripperFactory() httpf.RoundTripperFactory {
	return &noopRoundTripperFactory{}
}
