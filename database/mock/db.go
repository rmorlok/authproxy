// Code generated by MockGen. DO NOT EDIT.
// Source: database/db.go

// Package mock is a generated GoMock package.
package mock

import (
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
	uuid "github.com/google/uuid"
	context "github.com/rmorlok/authproxy/context"
	database "github.com/rmorlok/authproxy/database"
	jwt "github.com/rmorlok/authproxy/jwt"
)

// MockDB is a mock of DB interface.
type MockDB struct {
	ctrl     *gomock.Controller
	recorder *MockDBMockRecorder
}

// MockDBMockRecorder is the mock recorder for MockDB.
type MockDBMockRecorder struct {
	mock *MockDB
}

// NewMockDB creates a new mock instance.
func NewMockDB(ctrl *gomock.Controller) *MockDB {
	mock := &MockDB{ctrl: ctrl}
	mock.recorder = &MockDBMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDB) EXPECT() *MockDBMockRecorder {
	return m.recorder
}

// CheckNonceValidAndMarkUsed mocks base method.
func (m *MockDB) CheckNonceValidAndMarkUsed(ctx context.Context, nonce uuid.UUID, retainRecordUntil time.Time) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CheckNonceValidAndMarkUsed", ctx, nonce, retainRecordUntil)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CheckNonceValidAndMarkUsed indicates an expected call of CheckNonceValidAndMarkUsed.
func (mr *MockDBMockRecorder) CheckNonceValidAndMarkUsed(ctx, nonce, retainRecordUntil interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CheckNonceValidAndMarkUsed", reflect.TypeOf((*MockDB)(nil).CheckNonceValidAndMarkUsed), ctx, nonce, retainRecordUntil)
}

// CreateActor mocks base method.
func (m *MockDB) CreateActor(ctx context.Context, actor *database.Actor) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateActor", ctx, actor)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateActor indicates an expected call of CreateActor.
func (mr *MockDBMockRecorder) CreateActor(ctx, actor interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateActor", reflect.TypeOf((*MockDB)(nil).CreateActor), ctx, actor)
}

// CreateConnection mocks base method.
func (m *MockDB) CreateConnection(ctx context.Context, c *database.Connection) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CreateConnection", ctx, c)
	ret0, _ := ret[0].(error)
	return ret0
}

// CreateConnection indicates an expected call of CreateConnection.
func (mr *MockDBMockRecorder) CreateConnection(ctx, c interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CreateConnection", reflect.TypeOf((*MockDB)(nil).CreateConnection), ctx, c)
}

// EnumerateOAuth2TokensExpiringWithin mocks base method.
func (m *MockDB) EnumerateOAuth2TokensExpiringWithin(ctx context.Context, duration time.Duration, callback func([]*database.OAuth2TokenWithConnection, bool) (bool, error)) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EnumerateOAuth2TokensExpiringWithin", ctx, duration, callback)
	ret0, _ := ret[0].(error)
	return ret0
}

// EnumerateOAuth2TokensExpiringWithin indicates an expected call of EnumerateOAuth2TokensExpiringWithin.
func (mr *MockDBMockRecorder) EnumerateOAuth2TokensExpiringWithin(ctx, duration, callback interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnumerateOAuth2TokensExpiringWithin", reflect.TypeOf((*MockDB)(nil).EnumerateOAuth2TokensExpiringWithin), ctx, duration, callback)
}

// GetActor mocks base method.
func (m *MockDB) GetActor(ctx context.Context, id uuid.UUID) (*database.Actor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetActor", ctx, id)
	ret0, _ := ret[0].(*database.Actor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetActor indicates an expected call of GetActor.
func (mr *MockDBMockRecorder) GetActor(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetActor", reflect.TypeOf((*MockDB)(nil).GetActor), ctx, id)
}

// GetActorByExternalId mocks base method.
func (m *MockDB) GetActorByExternalId(ctx context.Context, externalId string) (*database.Actor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetActorByExternalId", ctx, externalId)
	ret0, _ := ret[0].(*database.Actor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetActorByExternalId indicates an expected call of GetActorByExternalId.
func (mr *MockDBMockRecorder) GetActorByExternalId(ctx, externalId interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetActorByExternalId", reflect.TypeOf((*MockDB)(nil).GetActorByExternalId), ctx, externalId)
}

// GetConnection mocks base method.
func (m *MockDB) GetConnection(ctx context.Context, id uuid.UUID) (*database.Connection, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetConnection", ctx, id)
	ret0, _ := ret[0].(*database.Connection)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetConnection indicates an expected call of GetConnection.
func (mr *MockDBMockRecorder) GetConnection(ctx, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetConnection", reflect.TypeOf((*MockDB)(nil).GetConnection), ctx, id)
}

// GetOAuth2Token mocks base method.
func (m *MockDB) GetOAuth2Token(ctx context.Context, connectionId uuid.UUID) (*database.OAuth2Token, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetOAuth2Token", ctx, connectionId)
	ret0, _ := ret[0].(*database.OAuth2Token)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetOAuth2Token indicates an expected call of GetOAuth2Token.
func (mr *MockDBMockRecorder) GetOAuth2Token(ctx, connectionId interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetOAuth2Token", reflect.TypeOf((*MockDB)(nil).GetOAuth2Token), ctx, connectionId)
}

// HasNonceBeenUsed mocks base method.
func (m *MockDB) HasNonceBeenUsed(ctx context.Context, nonce uuid.UUID) (bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HasNonceBeenUsed", ctx, nonce)
	ret0, _ := ret[0].(bool)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HasNonceBeenUsed indicates an expected call of HasNonceBeenUsed.
func (mr *MockDBMockRecorder) HasNonceBeenUsed(ctx, nonce interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HasNonceBeenUsed", reflect.TypeOf((*MockDB)(nil).HasNonceBeenUsed), ctx, nonce)
}

// InsertOAuth2Token mocks base method.
func (m *MockDB) InsertOAuth2Token(ctx context.Context, connectionId uuid.UUID, refreshedFrom *uuid.UUID, encryptedRefreshToken, encryptedAccessToken string, accessTokenExpiresAt *time.Time, scopes string) (*database.OAuth2Token, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InsertOAuth2Token", ctx, connectionId, refreshedFrom, encryptedRefreshToken, encryptedAccessToken, accessTokenExpiresAt, scopes)
	ret0, _ := ret[0].(*database.OAuth2Token)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// InsertOAuth2Token indicates an expected call of InsertOAuth2Token.
func (mr *MockDBMockRecorder) InsertOAuth2Token(ctx, connectionId, refreshedFrom, encryptedRefreshToken, encryptedAccessToken, accessTokenExpiresAt, scopes interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InsertOAuth2Token", reflect.TypeOf((*MockDB)(nil).InsertOAuth2Token), ctx, connectionId, refreshedFrom, encryptedRefreshToken, encryptedAccessToken, accessTokenExpiresAt, scopes)
}

// ListActorsBuilder mocks base method.
func (m *MockDB) ListActorsBuilder() database.ListActorsBuilder {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListActorsBuilder")
	ret0, _ := ret[0].(database.ListActorsBuilder)
	return ret0
}

// ListActorsBuilder indicates an expected call of ListActorsBuilder.
func (mr *MockDBMockRecorder) ListActorsBuilder() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListActorsBuilder", reflect.TypeOf((*MockDB)(nil).ListActorsBuilder))
}

// ListActorsFromCursor mocks base method.
func (m *MockDB) ListActorsFromCursor(ctx context.Context, cursor string) (database.ListActorsExecutor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListActorsFromCursor", ctx, cursor)
	ret0, _ := ret[0].(database.ListActorsExecutor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListActorsFromCursor indicates an expected call of ListActorsFromCursor.
func (mr *MockDBMockRecorder) ListActorsFromCursor(ctx, cursor interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListActorsFromCursor", reflect.TypeOf((*MockDB)(nil).ListActorsFromCursor), ctx, cursor)
}

// ListConnectionsBuilder mocks base method.
func (m *MockDB) ListConnectionsBuilder() database.ListConnectionsBuilder {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListConnectionsBuilder")
	ret0, _ := ret[0].(database.ListConnectionsBuilder)
	return ret0
}

// ListConnectionsBuilder indicates an expected call of ListConnectionsBuilder.
func (mr *MockDBMockRecorder) ListConnectionsBuilder() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListConnectionsBuilder", reflect.TypeOf((*MockDB)(nil).ListConnectionsBuilder))
}

// ListConnectionsFromCursor mocks base method.
func (m *MockDB) ListConnectionsFromCursor(ctx context.Context, cursor string) (database.ListConnectionsExecutor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ListConnectionsFromCursor", ctx, cursor)
	ret0, _ := ret[0].(database.ListConnectionsExecutor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ListConnectionsFromCursor indicates an expected call of ListConnectionsFromCursor.
func (mr *MockDBMockRecorder) ListConnectionsFromCursor(ctx, cursor interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListConnectionsFromCursor", reflect.TypeOf((*MockDB)(nil).ListConnectionsFromCursor), ctx, cursor)
}

// Migrate mocks base method.
func (m *MockDB) Migrate(ctx context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Migrate", ctx)
	ret0, _ := ret[0].(error)
	return ret0
}

// Migrate indicates an expected call of Migrate.
func (mr *MockDBMockRecorder) Migrate(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Migrate", reflect.TypeOf((*MockDB)(nil).Migrate), ctx)
}

// Ping mocks base method.
func (m *MockDB) Ping(ctx context.Context) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Ping", ctx)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Ping indicates an expected call of Ping.
func (mr *MockDBMockRecorder) Ping(ctx interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Ping", reflect.TypeOf((*MockDB)(nil).Ping), ctx)
}

// UpsertActor mocks base method.
func (m *MockDB) UpsertActor(ctx context.Context, actor *jwt.Actor) (*database.Actor, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "UpsertActor", ctx, actor)
	ret0, _ := ret[0].(*database.Actor)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// UpsertActor indicates an expected call of UpsertActor.
func (mr *MockDBMockRecorder) UpsertActor(ctx, actor interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "UpsertActor", reflect.TypeOf((*MockDB)(nil).UpsertActor), ctx, actor)
}
