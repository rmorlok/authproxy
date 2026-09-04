package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

func (s *service) getConnection(ctx context.Context, id apid.ID) (*connection, error) {
	logger := aplog.NewBuilder(s.logger).
		WithConnectionId(id).
		Build()

	logger.Debug("getting connection")
	dbConn, err := s.db.GetConnection(ctx, id)
	if err != nil {
		if errors.Is(database.ErrNotFound, err) {
			logger.Info("connection not found", "error", err)
			return nil, iface.ErrConnectionNotFound
		}

		logger.Error("failed to get connection", "error", err)
		return nil, err
	}

	return s.getConnectionForDb(ctx, dbConn)
}

func (s *service) GetConnection(ctx context.Context, id apid.ID) (iface.Connection, error) {
	return s.getConnection(ctx, id)
}

// resolveConnectionReference resolves a connection reference to a connection.
// This is used by object references within other resources.
func (s *service) resolveConnectionReference(
	ctx context.Context,
	ref meta.ObjectReference,
) (iface.Connection, error) {
	var byID iface.Connection
	if ref.HasID() {
		id, err := apid.Parse(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid connection reference: %v", ErrInvalidArgument, err)
		}
		byID, err = s.GetConnection(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ref.HasNamespacedName() {
			return byID, nil
		}
	}

	page := s.ListConnectionsBuilder().
		ForNamespaceMatcher(ref.Namespace).
		ForName(ref.Name).
		Limit(2).
		FetchPage(ctx)
	if page.Error != nil {
		return nil, page.Error
	}
	if len(page.Results) == 0 {
		return nil, ErrNotFound
	}
	if len(page.Results) > 1 {
		return nil, fmt.Errorf("%w: connection reference %q/%q is ambiguous", ErrInvalidArgument, ref.Namespace, ref.Name)
	}
	byName := page.Results[0]
	if byID != nil && byID.GetId() != byName.GetId() {
		return nil, fmt.Errorf("%w: connection reference id %q does not match %q/%q", ErrInvalidArgument, ref.ID, ref.Namespace, ref.Name)
	}
	return byName, nil
}
