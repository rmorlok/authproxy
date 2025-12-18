package core

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) getConnection(ctx context.Context, id uuid.UUID) (*connection, error) {
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

func (s *service) GetConnection(ctx context.Context, id uuid.UUID) (iface.Connection, error) {
	return s.getConnection(ctx, id)
}
