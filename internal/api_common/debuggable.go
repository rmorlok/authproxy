package api_common

// Debuggable is the interface required for response methods in this packet. This interface would normally be filled
// by passing the config.C
type Debuggable interface {
	IsDebugMode() bool
}

type mockDebuggable struct {
	debug bool
}

func (m *mockDebuggable) IsDebugMode() bool { return m.debug }

func NewMockDebuggable(debug bool) Debuggable {
	return &mockDebuggable{debug: debug}
}
