package mock

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
)

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

	if m.ExpectedState != "" && c.GetState() != m.ExpectedState {
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
