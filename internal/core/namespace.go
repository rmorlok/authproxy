package core

import (
	"log/slog"
	"time"

	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

// Namespace is the core abstraction around namespaces.
type Namespace struct {
	database.Namespace

	s      *service
	logger *slog.Logger
}

func wrapNamespace(ns database.Namespace, s *service) *Namespace {
	return &Namespace{
		Namespace: ns,
		s:         s,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(ns.Path).
			Build(),
	}
}

func (ns *Namespace) GetNamespace() string {
	return ns.Path
}

func (ns *Namespace) GetPath() string {
	return ns.Path
}

func (ns *Namespace) GetState() database.NamespaceState {
	return ns.State
}

func (ns *Namespace) GetCreatedAt() time.Time {
	return ns.CreatedAt
}

func (ns *Namespace) GetUpdatedAt() time.Time {
	return ns.UpdatedAt
}

func (ns *Namespace) GetLabels() map[string]string {
	return ns.Labels
}

func (ns *Namespace) Logger() *slog.Logger {
	return ns.logger
}

var _ iface.Namespace = (*Namespace)(nil)
var _ aplog.HasLogger = (*Namespace)(nil)
