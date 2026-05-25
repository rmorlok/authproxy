package core

import (
	"log/slog"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	rlschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

// RateLimit is the core abstraction wrapping a database.RateLimit.
type RateLimit struct {
	database.RateLimit

	s      *service
	logger *slog.Logger
}

func wrapRateLimit(rl database.RateLimit, s *service) *RateLimit {
	return &RateLimit{
		RateLimit: rl,
		s:         s,
		logger: aplog.NewBuilder(s.logger).
			WithNamespace(rl.Namespace).
			Build(),
	}
}

func (r *RateLimit) GetId() apid.ID                    { return r.Id }
func (r *RateLimit) GetNamespace() string              { return r.Namespace }
func (r *RateLimit) GetDefinition() rlschema.RateLimit { return r.Definition }
func (r *RateLimit) GetLabels() map[string]string      { return r.Labels }
func (r *RateLimit) GetAnnotations() map[string]string { return r.Annotations }
func (r *RateLimit) GetCreatedAt() time.Time           { return r.CreatedAt }
func (r *RateLimit) GetUpdatedAt() time.Time           { return r.UpdatedAt }
func (r *RateLimit) Logger() *slog.Logger              { return r.logger }

var _ iface.RateLimit = (*RateLimit)(nil)
var _ aplog.HasLogger = (*RateLimit)(nil)
