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
