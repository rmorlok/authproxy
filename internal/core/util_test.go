package core

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	mockAsynq "github.com/rmorlok/authproxy/internal/apasynq/mock"
	"github.com/rmorlok/authproxy/internal/apid"
	mockLog "github.com/rmorlok/authproxy/internal/aplog/mock"
	"github.com/rmorlok/authproxy/internal/apredis/mock"
	"github.com/rmorlok/authproxy/internal/core/iface"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
	mockE "github.com/rmorlok/authproxy/internal/encrypt/mock"
	mockF "github.com/rmorlok/authproxy/internal/httpf/mock"
	"github.com/stretchr/testify/require"

	"github.com/rmorlok/authproxy/internal/database"
)

func FullMockService(tb testing.TB, ctrl *gomock.Controller) (*service, *mockDb.MockDB, *mock.MockClient, *mockF.MockF, *mockAsynq.MockClient, *mockE.MockE) {
	db := mockDb.NewMockDB(ctrl)
	ac := mockAsynq.NewMockClient(ctrl)
	r := mock.NewMockClient(ctrl)
	h := mockF.NewMockF(ctrl)
	encrypt := mockE.NewMockE(ctrl)
	logger, _ := mockLog.NewTestLogger(tb)

	s := &service{
		cfg:     nil,
		db:      db,
		encrypt: encrypt,
		ac:      ac,
		httpf:   h,
		r:       r,
		logger:  logger,
	}
	// Build a registry matching production wiring so call sites that resolve
	// through getAuthMethodFactory find a real factory (backed by the same
	// mock db / encrypt that the test set up). Tests that want a mocked
	// factory swap it into authMethodFactories themselves.
	s.authMethodFactories = s.buildAuthMethodFactories()
	return s, db, r, h, ac, encrypt
}

func TestGetConnectorGenerationIdsForConnections(t *testing.T) {
	u1 := apid.MustParse("cxr_test1111111111aa")
	u2 := apid.MustParse("cxr_test2222222222aa")

	tests := []struct {
		name        string
		connections []database.Connection
		expected    []iface.ConnectorGenerationId
	}{
		{
			name:        "empty input",
			connections: nil,
			expected:    []iface.ConnectorGenerationId{},
		},
		{
			name:        "single connection",
			connections: []database.Connection{{ConnectorId: u1, ConnectorGeneration: 1}},
			expected:    []iface.ConnectorGenerationId{{Id: u1, Generation: 1}},
		},
		{
			name: "multiple unique connections",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorGeneration: 1},
				{ConnectorId: u2, ConnectorGeneration: 2},
			},
			expected: []iface.ConnectorGenerationId{
				{Id: u1, Generation: 1},
				{Id: u2, Generation: 2},
			},
		},
		{
			name: "duplicate connections are deduplicated",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorGeneration: 1},
				{ConnectorId: u1, ConnectorGeneration: 1},
			},
			expected: []iface.ConnectorGenerationId{
				{Id: u1, Generation: 1},
			},
		},
		{
			name: "different versions considered unique",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorGeneration: 1},
				{ConnectorId: u1, ConnectorGeneration: 2},
			},
			expected: []iface.ConnectorGenerationId{
				{Id: u1, Generation: 1},
				{Id: u1, Generation: 2},
			},
		},
		{
			name: "different connector IDs considered unique",
			connections: []database.Connection{
				{ConnectorId: u1, ConnectorGeneration: 1},
				{ConnectorId: u2, ConnectorGeneration: 1},
			},
			expected: []iface.ConnectorGenerationId{
				{Id: u1, Generation: 1},
				{Id: u2, Generation: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetConnectorGenerationIdsForConnections(tt.connections)
			if !compareResults(got, tt.expected) {
				t.Errorf("GetConnectorGenerationIdsForConnections() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func compareResults(got, expected []iface.ConnectorGenerationId) bool {
	if len(got) != len(expected) {
		return false
	}
	gotMap := make(map[iface.ConnectorGenerationId]struct{}, len(got))
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

func TestMapDatabaseError(t *testing.T) {
	randomErr := errors.New("some error")
	require.Equal(t, randomErr, mapDatabaseError(randomErr))
	require.ErrorIs(t, mapDatabaseError(database.ErrNotFound), ErrNotFound)
}
