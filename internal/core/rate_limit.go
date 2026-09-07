package core

import (
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
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

// rateLimitResourceFromDatabase converts the flat persistence envelope into
// the canonical resource at the core boundary. The definition column already
// stores the canonical spec, so no API DTO enters persistence or enforcement.
func rateLimitResourceFromDatabase(rl database.RateLimit) *rlschema.RateLimit {
	createdAt := rl.CreatedAt
	updatedAt := rl.UpdatedAt
	return &rlschema.RateLimit{
		TypeMeta: meta.NewTypeMeta(rlschema.RateLimitKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:          rl.Id.String(),
			Name:        rl.Name,
			Namespace:   rl.Namespace,
			Labels:      maps.Clone(map[string]string(rl.Labels)),
			Annotations: maps.Clone(map[string]string(rl.Annotations)),
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}),
		Spec: rl.Definition.Clone(),
		Status: &rlschema.RateLimitStatus{
			EffectiveMode: rl.Definition.EffectiveMode(),
		},
	}
}

func databaseRateLimitFromResource(resource *rlschema.RateLimit, id apid.ID) (*database.RateLimit, error) {
	if resource == nil {
		return nil, fmt.Errorf("rate limit is required")
	}
	return &database.RateLimit{
		Id:          id,
		Namespace:   resource.Metadata.Namespace,
		Name:        resource.Metadata.Name,
		Definition:  resource.Spec.Clone(),
		Labels:      database.Labels(maps.Clone(resource.Metadata.Labels)),
		Annotations: database.Annotations(maps.Clone(resource.Metadata.Annotations)),
	}, nil
}

func (r *RateLimit) GetId() apid.ID                    { return r.Id }
func (r *RateLimit) GetNamespace() string              { return r.Namespace }
func (r *RateLimit) GetName() scommon.ResourceName     { return r.Name }
func (r *RateLimit) GetSpec() rlschema.RateLimitSpec   { return r.Definition.Clone() }
func (r *RateLimit) GetLabels() map[string]string      { return r.Labels }
func (r *RateLimit) GetAnnotations() map[string]string { return r.Annotations }
func (r *RateLimit) GetCreatedAt() time.Time           { return r.CreatedAt }
func (r *RateLimit) GetUpdatedAt() time.Time           { return r.UpdatedAt }
func (r *RateLimit) GetResource() *rlschema.RateLimit {
	return rateLimitResourceFromDatabase(r.RateLimit)
}
func (r *RateLimit) Logger() *slog.Logger { return r.logger }

var _ iface.RateLimit = (*RateLimit)(nil)
var _ aplog.HasLogger = (*RateLimit)(nil)
