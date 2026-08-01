package mock

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
)

type ConnectorWithDefinitionMatcher struct {
	ExpectedId      apid.ID
	ExpectedVersion uint64
}

func (m ConnectorWithDefinitionMatcher) Matches(x interface{}) bool {
	cv, ok := x.(database.ConnectorWithDefinition)
	if !ok {
		return false
	}

	return cv.Id == m.ExpectedId && cv.Version == m.ExpectedVersion
}

func (m ConnectorWithDefinitionMatcher) String() string {
	return fmt.Sprintf("is ConnectorWithDefinition with ID=%s, Version=%d", m.ExpectedId, m.ExpectedVersion)
}

type ConnectionMatcher struct {
	ExpectedId apid.ID
}

func (m ConnectionMatcher) Matches(x interface{}) bool {
	c, ok := x.(database.Connection)
	if !ok {
		return false
	}

	return c.Id == m.ExpectedId
}

func (m ConnectionMatcher) String() string {
	return fmt.Sprintf("is Connection with ID=%s", m.ExpectedId)
}

type NamespaceMatcher struct {
	ExpectedPath  string
	ExpectedState database.NamespaceState
}

func (m NamespaceMatcher) Matches(x interface{}) bool {
	c, ok := x.(*database.Namespace)
	if !ok {
		return false
	}

	if m.ExpectedPath != "" && c.Path != m.ExpectedPath {
		return false
	}

	if m.ExpectedState != "" && c.State != m.ExpectedState {
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
