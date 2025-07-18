// Code generated by MockGen. DO NOT EDIT.
// Source: client_interface.go

// Package mock is a generated GoMock package.
package mock

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	asynq "github.com/hibiken/asynq"
)

// MockClient is a mock of Client interface.
type MockClient struct {
	ctrl     *gomock.Controller
	recorder *MockClientMockRecorder
}

// MockClientMockRecorder is the mock recorder for MockClient.
type MockClientMockRecorder struct {
	mock *MockClient
}

// NewMockClient creates a new mock instance.
func NewMockClient(ctrl *gomock.Controller) *MockClient {
	mock := &MockClient{ctrl: ctrl}
	mock.recorder = &MockClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClient) EXPECT() *MockClientMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockClient) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockClientMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockClient)(nil).Close))
}

// Enqueue mocks base method.
func (m *MockClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{task}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "Enqueue", varargs...)
	ret0, _ := ret[0].(*asynq.TaskInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Enqueue indicates an expected call of Enqueue.
func (mr *MockClientMockRecorder) Enqueue(task interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{task}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Enqueue", reflect.TypeOf((*MockClient)(nil).Enqueue), varargs...)
}

// EnqueueContext mocks base method.
func (m *MockClient) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	m.ctrl.T.Helper()
	varargs := []interface{}{ctx, task}
	for _, a := range opts {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "EnqueueContext", varargs...)
	ret0, _ := ret[0].(*asynq.TaskInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EnqueueContext indicates an expected call of EnqueueContext.
func (mr *MockClientMockRecorder) EnqueueContext(ctx, task interface{}, opts ...interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]interface{}{ctx, task}, opts...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EnqueueContext", reflect.TypeOf((*MockClient)(nil).EnqueueContext), varargs...)
}

// Ping mocks base method.
func (m *MockClient) Ping() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Ping")
	ret0, _ := ret[0].(error)
	return ret0
}

// Ping indicates an expected call of Ping.
func (mr *MockClientMockRecorder) Ping() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Ping", reflect.TypeOf((*MockClient)(nil).Ping))
}
