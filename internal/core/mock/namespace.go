package mock

import (
	"fmt"

	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	nsschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type Namespace struct {
	Path        string
	Name        common.ResourceName
	State       database.NamespaceState
	KeyId       *apid.ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Labels      map[string]string
	Annotations map[string]string
}

func (m *Namespace) GetPath() string {
	return m.Path
}

func (m *Namespace) GetName() common.ResourceName {
	if m.Name != "" {
		return m.Name
	}
	return nsschema.NameFromPath(m.Path)
}

func (m *Namespace) GetState() nsschema.NamespaceState {
	return nsschema.NamespaceState(m.State)
}

func (m *Namespace) GetCreatedAt() time.Time {
	return m.CreatedAt
}

func (m *Namespace) GetUpdatedAt() time.Time {
	return m.UpdatedAt
}

func (m *Namespace) GetKeyId() *apid.ID {
	return m.KeyId
}

func (m *Namespace) GetLabels() map[string]string {
	return m.Labels
}

func (m *Namespace) GetAnnotations() map[string]string {
	return m.Annotations
}

func (m *Namespace) GetResource() *nsschema.Namespace {
	resource, err := nsschema.NewNamespaceResourceForPath(m.Path)
	if err != nil {
		return nil
	}
	resource.Metadata.Labels = m.Labels
	resource.Metadata.Annotations = m.Annotations
	resource.Metadata.CreatedAt = &m.CreatedAt
	resource.Metadata.UpdatedAt = &m.UpdatedAt
	resource.Metadata = meta.NormalizeObjectMeta(resource.Metadata)
	resource.Status = &nsschema.NamespaceStatus{State: nsschema.NamespaceState(m.State)}
	if m.KeyId != nil {
		resource.Spec.EncryptionKeyRef = nsschema.NewEncryptionKeyReference(*m.KeyId)
	}
	return resource
}

var _ iface.Namespace = (*Namespace)(nil)

type NamespaceMatcher struct {
	ExpectedPath  string
	ExpectedState database.NamespaceState
}

func (m NamespaceMatcher) Matches(x interface{}) bool {
	c, ok := x.(iface.Namespace)
	if !ok {
		return false
	}

	if m.ExpectedPath != "" && c.GetPath() != m.ExpectedPath {
		return false
	}

	if m.ExpectedState != "" && c.GetState() != nsschema.NamespaceState(m.ExpectedState) {
		return false
	}

	return true
}

func (m NamespaceMatcher) String() string {
	if m.ExpectedPath == "" && m.ExpectedState == "" {
		return "is Namespace"
	} else if m.ExpectedPath == "" {
		return fmt.Sprintf("is Namespace with State=%s", m.ExpectedState)
	} else {
		return fmt.Sprintf("is Namespace with Path=%s and State=%s", m.ExpectedPath, m.ExpectedState)
	}
}
