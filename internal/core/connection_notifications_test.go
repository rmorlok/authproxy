package core

import (
	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	mockDb "github.com/rmorlok/authproxy/internal/database/mock"
)

func expectResolveRequiredActionNotification(
	db *mockDb.MockDB,
	connectionID apid.ID,
	keyPart string,
) *gomock.Call {
	return db.EXPECT().
		ResolveNotificationsForResourceKeys(
			gomock.Any(),
			"connection",
			connectionID,
			[]string{connectionRequiredActionNotificationKey(connectionID, keyPart)},
		).
		Return(nil)
}
