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
	nschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
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

// namespaceResourceFromDatabase converts the flat persistence model at the
// core boundary. Database rows deliberately remain independent of the public
// resource envelope.
func namespaceResourceFromDatabase(ns database.Namespace) *nschema.Namespace {
	result, err := nschema.NewNamespaceResourceForPath(ns.Path)
	if err != nil {
		return nil
	}
	result.Metadata.Labels = maps.Clone(map[string]string(ns.Labels))
	result.Metadata.Annotations = maps.Clone(map[string]string(ns.Annotations))
	result.Metadata.CreatedAt = &ns.CreatedAt
	result.Metadata.UpdatedAt = &ns.UpdatedAt
	result.Metadata = meta.NormalizeObjectMeta(result.Metadata)
	result.Status = &nschema.NamespaceStatus{
		State: nschema.NamespaceState(ns.State),
	}
	if ns.KeyId != nil {
		result.Spec.EncryptionKeyRef = nschema.NewEncryptionKeyReference(*ns.KeyId)
	}
	return result
}

// databaseNamespaceFromResource converts desired resource data into the flat
// database model used to create a namespace.
func databaseNamespaceFromResource(resource *nschema.Namespace) (*database.Namespace, error) {
	if resource == nil {
		return nil, fmt.Errorf("namespace is required")
	}
	path, err := nschema.PathFromMetadata(resource.Metadata)
	if err != nil {
		return nil, err
	}

	result := &database.Namespace{
		Path:        path,
		State:       database.NamespaceStateActive,
		Labels:      database.Labels(maps.Clone(resource.Metadata.Labels)),
		Annotations: database.Annotations(maps.Clone(resource.Metadata.Annotations)),
	}
	if ref := resource.Spec.EncryptionKeyRef; ref != nil {
		id, err := nschema.EncryptionKeyID(ref)
		if err != nil {
			return nil, fmt.Errorf("invalid namespace encryption key reference: %w", err)
		}
		result.KeyId = id
	}
	return result, nil
}

func (ns *Namespace) GetNamespace() string {
	return ns.Path
}

func (ns *Namespace) GetPath() string {
	return ns.Path
}

func (ns *Namespace) GetName() scommon.ResourceName {
	return nschema.NameFromPath(ns.Path)
}

func (ns *Namespace) GetState() nschema.NamespaceState {
	return nschema.NamespaceState(ns.State)
}

func (ns *Namespace) GetCreatedAt() time.Time {
	return ns.CreatedAt
}

func (ns *Namespace) GetUpdatedAt() time.Time {
	return ns.UpdatedAt
}

func (ns *Namespace) GetKeyId() *apid.ID {
	return ns.KeyId
}

func (ns *Namespace) GetLabels() map[string]string {
	return ns.Labels
}

func (ns *Namespace) GetAnnotations() map[string]string {
	return ns.Annotations
}

func (ns *Namespace) GetResource() *nschema.Namespace {
	return namespaceResourceFromDatabase(ns.Namespace)
}

func (ns *Namespace) Logger() *slog.Logger {
	return ns.logger
}

var _ iface.Namespace = (*Namespace)(nil)
var _ aplog.HasLogger = (*Namespace)(nil)
