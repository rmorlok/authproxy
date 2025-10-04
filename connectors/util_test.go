package connectors

import (
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	mockAsynq "github.com/rmorlok/authproxy/apasynq/mock"
	mockLog "github.com/rmorlok/authproxy/aplog/mock"
	mockR "github.com/rmorlok/authproxy/apredis/mock"
	"github.com/rmorlok/authproxy/connectors/interface"
	mockDb "github.com/rmorlok/authproxy/database/mock"
	mockE "github.com/rmorlok/authproxy/encrypt/mock"
	mockF "github.com/rmorlok/authproxy/httpf/mock"
	"testing"

	"github.com/rmorlok/authproxy/database"
)

func FullMockService(tb testing.TB, ctrl *gomock.Controller) (*service, *mockDb.MockDB, *mockR.MockClient, *mockF.MockF, *mockAsynq.MockClient, *mockE.MockE) {
	db := mockDb.NewMockDB(ctrl)
	ac := mockAsynq.NewMockClient(ctrl)
	r := mockR.NewMockClient(ctrl)
	h := mockF.NewMockF(ctrl)
	encrypt := mockE.NewMockE(ctrl)
	logger, _ := mockLog.NewTestLogger(tb)

	return &service{
		cfg:     nil,
		db:      db,
		encrypt: encrypt,
		ac:      ac,
		httpf:   h,
		r:       r,
		logger:  logger,
	}, db, r, h, ac, encrypt
}

func TestGetConnectorVersionIdsForConnections(t *testing.T) {
	u1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	u2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name        string
		connections []database.Connection
		expected    []_interface.ConnectorVersionId
	}{
		{
			name:        "empty input",
			connections: nil,
			expected:    []_interface.ConnectorVersionId{},
		},
		{
			name:        "single connection",
			connections: []database.Connection{{ConnectorId: u1, ConnectorVersion: 1}},
			expected:    []_interface.ConnectorVersionId{{Id: u1, Version: 1}},
		},
		{
			name: "multiple unique connections",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorVersion: 1},
				{ConnectorId: u2, ConnectorVersion: 2},
			},
			expected: []_interface.ConnectorVersionId{
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
			expected: []_interface.ConnectorVersionId{
				{Id: u1, Version: 1},
			},
		},
		{
			name: "different versions considered unique",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorVersion: 1},
				{ConnectorId: u1, ConnectorVersion: 2},
			},
			expected: []_interface.ConnectorVersionId{
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
			expected: []_interface.ConnectorVersionId{
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

func compareResults(got, expected []_interface.ConnectorVersionId) bool {
	if len(got) != len(expected) {
		return false
	}
	gotMap := make(map[_interface.ConnectorVersionId]struct{}, len(got))
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
