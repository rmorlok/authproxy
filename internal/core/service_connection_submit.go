package core

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rmorlok/authproxy/internal/api_common"
	"github.com/rmorlok/authproxy/internal/core/iface"
)

// SubmitForm handles form data submission for a connection setup flow. This is a shell implementation
// that will be fully implemented when form-based connector types are added.
func (c *connection) SubmitForm(ctx context.Context, req iface.SubmitConnectionRequest) (iface.InitiateConnectionResponse, error) {
	return nil, api_common.NewHttpStatusErrorBuilder().
		WithStatus(http.StatusNotImplemented).
		WithPublicErr(fmt.Errorf("form submission is not yet implemented")).
		BuildStatusError()
}
