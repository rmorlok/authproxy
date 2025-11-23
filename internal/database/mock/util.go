package mock

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
)

type ConnectorVersionMatcher struct {
	ExpectedId      uuid.UUID
	ExpectedVersion uint64
}

func (m ConnectorVersionMatcher) Matches(x interface{}) bool {
	cv, ok := x.(database.ConnectorVersion)
	if !ok {
		return false
	}

	return cv.ID == m.ExpectedId && cv.Version == m.ExpectedVersion
}

func (m ConnectorVersionMatcher) String() string {
	return fmt.Sprintf("is ConnectorVersion with ID=%s, Version=%d", m.ExpectedId, m.ExpectedVersion)
}

type ConnectionMatcher struct {
	ExpectedId uuid.UUID
}

func (m ConnectionMatcher) Matches(x interface{}) bool {
	c, ok := x.(database.Connection)
	if !ok {
		return false
	}

	return c.ID == m.ExpectedId
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
