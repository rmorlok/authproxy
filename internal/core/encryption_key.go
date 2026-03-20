package core

import (
	"log/slog"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// EncryptionKey is the core abstraction around encryption keys.
type EncryptionKey struct {
	database.EncryptionKey

	s      *service
	logger *slog.Logger
}

func wrapEncryptionKey(ek database.EncryptionKey, s *service) *EncryptionKey {
	return &EncryptionKey{
		EncryptionKey: ek,
		s:             s,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(ek.Namespace).
			Build(),
	}
}

func (ek *EncryptionKey) GetId() apid.ID {
	return ek.Id
}

func (ek *EncryptionKey) GetNamespace() string {
	return ek.Namespace
}

func (ek *EncryptionKey) GetState() database.EncryptionKeyState {
	return ek.State
}

func (ek *EncryptionKey) GetCreatedAt() time.Time {
	return ek.CreatedAt
}

func (ek *EncryptionKey) GetUpdatedAt() time.Time {
	return ek.UpdatedAt
}

func (ek *EncryptionKey) GetLabels() map[string]string {
	return ek.Labels
}

func (ek *EncryptionKey) GetAnnotations() map[string]string {
	return ek.Annotations
}

func (ek *EncryptionKey) Logger() *slog.Logger {
	return ek.logger
}

var _ iface.EncryptionKey = (*EncryptionKey)(nil)
var _ aplog.HasLogger = (*EncryptionKey)(nil)
