package core

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

func (s *service) EnsureNamespaceAncestorPath(ctx context.Context, targetNamespace string) (iface.Namespace, error) {
	if err := common.ValidateNamespacePath(targetNamespace); err != nil {
		return nil, err
	}

	var err error
	var final *database.Namespace
	for _, ns := range common.SplitNamespacePathToPrefixes(targetNamespace) {
		final, err = s.db.GetNamespace(ctx, ns)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				final = &database.Namespace{
					Path:      ns,
					State:     database.NamespaceStateActive,
					CreatedAt: apctx.GetClock(ctx).Now(),
					UpdatedAt: apctx.GetClock(ctx).Now(),
				}
				err := s.db.CreateNamespace(ctx, final)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
	}

	if final == nil {
		return nil, errors.New("failed to create namespace")
	}

	return wrapNamespace(*final, s), nil
}
