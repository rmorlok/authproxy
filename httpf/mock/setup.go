package mock

import (
	"github.com/golang/mock/gomock"
	genmock "gopkg.in/h2non/gentleman-mock.v2"
	"gopkg.in/h2non/gentleman.v2"
	"gopkg.in/h2non/gock.v1"
)

type cleanuper interface {
	Cleanup(func())
}

func NewFactoryWithMockingClient(ctrl *gomock.Controller) *MockF {
	cli := gentleman.New()
	cli.Use(genmock.Plugin)

	if c, ok := ctrl.T.(cleanuper); ok {
		// Help protect against leaking mocks
		c.Cleanup(func() {
			gock.Off()
		})
	}

	h := NewMockF(ctrl)

	h.
		EXPECT().
		ForRequestInfo(gomock.Any()).
		Return(h).
		AnyTimes()

	h.
		EXPECT().
		ForRequestType(gomock.Any()).
		Return(h).
		AnyTimes()

	h.
		EXPECT().
		ForConnectorVersion(gomock.Any()).
		Return(h).
		AnyTimes()

	h.
		EXPECT().
		ForConnection(gomock.Any()).
		Return(h).
		AnyTimes()

	h.
		EXPECT().
		New().
		Return(cli).
		AnyTimes()

	return h
}
