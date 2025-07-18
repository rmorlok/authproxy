package connectors

import (
	"github.com/google/uuid"
	"testing"

	"github.com/rmorlok/authproxy/database"
)

func TestGetConnectorVersionIdsForConnections(t *testing.T) {
	u1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	u2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name        string
		connections []database.Connection
		expected    []ConnectorVersionId
	}{
		{
			name:        "empty input",
			connections: nil,
			expected:    []ConnectorVersionId{},
		},
		{
			name:        "single connection",
			connections: []database.Connection{{ConnectorId: u1, ConnectorVersion: 1}},
			expected:    []ConnectorVersionId{{Id: u1, Version: 1}},
		},
		{
			name: "multiple unique connections",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorVersion: 1},
				{ConnectorId: u2, ConnectorVersion: 2},
			},
			expected: []ConnectorVersionId{
				{Id: u1, Version: 1},
				{Id: u2, Version: 2},
			},
		},
		{
			name: "duplicate connections are deduplicated",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorVersion: 1},
				{ConnectorId: u1, ConnectorVersion: 1},
			},
			expected: []ConnectorVersionId{
				{Id: u1, Version: 1},
			},
		},
		{
			name: "different versions considered unique",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorVersion: 1},
				{ConnectorId: u1, ConnectorVersion: 2},
			},
			expected: []ConnectorVersionId{
				{Id: u1, Version: 1},
				{Id: u1, Version: 2},
			},
		},
		{
			name: "different connector IDs considered unique",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorVersion: 1},
				{ConnectorId: u2, ConnectorVersion: 1},
			},
			expected: []ConnectorVersionId{
				{Id: u1, Version: 1},
				{Id: u2, Version: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetConnectorVersionIdsForConnections(tt.connections)
			if !compareResults(got, tt.expected) {
				t.Errorf("GetConnectorVersionIdsForConnections() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func compareResults(got, expected []ConnectorVersionId) bool {
	if len(got) != len(expected) {
		return false
	}
	gotMap := make(map[ConnectorVersionId]struct{}, len(got))
	for _, id := range got {
		gotMap[id] = struct{}{}
	}
	for _, id := range expected {
		if _, found := gotMap[id]; !found {
			return false
		}
	}
	return true
}
