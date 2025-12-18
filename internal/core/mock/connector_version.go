package mock

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/core/iface"
)

type ConnectorVersionMatcher struct {
	ExpectedId      uuid.UUID
	ExpectedVersion uint64
}

func (m ConnectorVersionMatcher) Matches(x interface{}) bool {
	cv, ok := x.(iface.ConnectorVersion)
	if !ok {
		return false
	}

	return cv.GetID() == m.ExpectedId && cv.GetVersion() == m.ExpectedVersion
}

func (m ConnectorVersionMatcher) String() string {
	return fmt.Sprintf("is ConnectorVersion with ID=%s, Version=%d", m.ExpectedId, m.ExpectedVersion)
}
