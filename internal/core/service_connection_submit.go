package core

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
)

// SubmitConnection handles form data submission for a connection setup flow. This is a shell implementation
// that will be fully implemented when form-based connector types are added.
func (s *service) SubmitConnection(ctx context.Context, connectionId apid.ID, req iface.SubmitConnectionRequest) (iface.InitiateConnectionResponse, error) {
	// Verify the connection exists
	_, err := s.GetConnection(ctx, connectionId)
	if err != nil {
		return nil, err
	}

	return nil, api_common.NewHttpStatusErrorBuilder().
		WithStatus(http.StatusNotImplemented).
		WithPublicErr(fmt.Errorf("form submission is not yet implemented")).
		BuildStatusError()
}
