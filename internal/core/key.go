package core

import (
	"log/slog"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// Key is the core abstraction around encryption keys.
type Key struct {
	database.Key

	s      *service
	logger *slog.Logger
}

func wrapKey(ek database.Key, s *service) *Key {
	return &Key{
		Key: ek,
		s:   s,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(ek.Namespace).
			Build(),
	}
}

func (ek *Key) GetId() apid.ID {
	return ek.Id
}

func (ek *Key) GetNamespace() string {
	return ek.Namespace
}

func (ek *Key) GetState() database.KeyState {
	return ek.State
}

func (ek *Key) GetCreatedAt() time.Time {
	return ek.CreatedAt
}

func (ek *Key) GetUpdatedAt() time.Time {
	return ek.UpdatedAt
}

func (ek *Key) GetLabels() map[string]string {
	return ek.Labels
}

func (ek *Key) GetAnnotations() map[string]string {
	return ek.Annotations
}

func (ek *Key) Logger() *slog.Logger {
	return ek.logger
}

var _ iface.Key = (*Key)(nil)
var _ aplog.HasLogger = (*Key)(nil)
