// Code generated by MockGen. DO NOT EDIT.
// Source: encrypt/interface.go

// Package mock is a generated GoMock package.
package mock

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	context "github.com/rmorlok/authproxy/context"
	database "github.com/rmorlok/authproxy/database"
)

// MockE is a mock of E interface.
type MockE struct {
	ctrl     *gomock.Controller
	recorder *MockEMockRecorder
}

// MockEMockRecorder is the mock recorder for MockE.
type MockEMockRecorder struct {
	mock *MockE
}

// NewMockE creates a new mock instance.
func NewMockE(ctrl *gomock.Controller) *MockE {
	mock := &MockE{ctrl: ctrl}
	mock.recorder = &MockEMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockE) EXPECT() *MockEMockRecorder {
	return m.recorder
}

// DecryptForConnection mocks base method.
func (m *MockE) DecryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecryptForConnection", ctx, connection, data)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DecryptForConnection indicates an expected call of DecryptForConnection.
func (mr *MockEMockRecorder) DecryptForConnection(ctx, connection, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DecryptForConnection", reflect.TypeOf((*MockE)(nil).DecryptForConnection), ctx, connection, data)
}

// DecryptGlobal mocks base method.
func (m *MockE) DecryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecryptGlobal", ctx, data)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DecryptGlobal indicates an expected call of DecryptGlobal.
func (mr *MockEMockRecorder) DecryptGlobal(ctx, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DecryptGlobal", reflect.TypeOf((*MockE)(nil).DecryptGlobal), ctx, data)
}

// DecryptStringForConnection mocks base method.
func (m *MockE) DecryptStringForConnection(ctx context.Context, connection database.Connection, base64 string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecryptStringForConnection", ctx, connection, base64)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DecryptStringForConnection indicates an expected call of DecryptStringForConnection.
func (mr *MockEMockRecorder) DecryptStringForConnection(ctx, connection, base64 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DecryptStringForConnection", reflect.TypeOf((*MockE)(nil).DecryptStringForConnection), ctx, connection, base64)
}

// DecryptStringGlobal mocks base method.
func (m *MockE) DecryptStringGlobal(ctx context.Context, base64 string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DecryptStringGlobal", ctx, base64)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// DecryptStringGlobal indicates an expected call of DecryptStringGlobal.
func (mr *MockEMockRecorder) DecryptStringGlobal(ctx, base64 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DecryptStringGlobal", reflect.TypeOf((*MockE)(nil).DecryptStringGlobal), ctx, base64)
}

// EncryptForConnection mocks base method.
func (m *MockE) EncryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EncryptForConnection", ctx, connection, data)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EncryptForConnection indicates an expected call of EncryptForConnection.
func (mr *MockEMockRecorder) EncryptForConnection(ctx, connection, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EncryptForConnection", reflect.TypeOf((*MockE)(nil).EncryptForConnection), ctx, connection, data)
}

// EncryptGlobal mocks base method.
func (m *MockE) EncryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EncryptGlobal", ctx, data)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EncryptGlobal indicates an expected call of EncryptGlobal.
func (mr *MockEMockRecorder) EncryptGlobal(ctx, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EncryptGlobal", reflect.TypeOf((*MockE)(nil).EncryptGlobal), ctx, data)
}

// EncryptStringForConnection mocks base method.
func (m *MockE) EncryptStringForConnection(ctx context.Context, connection database.Connection, data string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EncryptStringForConnection", ctx, connection, data)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EncryptStringForConnection indicates an expected call of EncryptStringForConnection.
func (mr *MockEMockRecorder) EncryptStringForConnection(ctx, connection, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EncryptStringForConnection", reflect.TypeOf((*MockE)(nil).EncryptStringForConnection), ctx, connection, data)
}

// EncryptStringGlobal mocks base method.
func (m *MockE) EncryptStringGlobal(ctx context.Context, data string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EncryptStringGlobal", ctx, data)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EncryptStringGlobal indicates an expected call of EncryptStringGlobal.
func (mr *MockEMockRecorder) EncryptStringGlobal(ctx, data interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EncryptStringGlobal", reflect.TypeOf((*MockE)(nil).EncryptStringGlobal), ctx, data)
}
