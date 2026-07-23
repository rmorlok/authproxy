package core

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
)

func connectionRequiredActionNotificationKey(connectionID apid.ID, keyPart string) string {
	return fmt.Sprintf("connection:%s:%s", connectionID, keyPart)
}

func (c *connection) resolveRequiredActionNotifications(ctx context.Context, keyParts ...string) error {
	keys := make([]string, 0, len(keyParts))
	for _, keyPart := range keyParts {
		keys = append(keys, connectionRequiredActionNotificationKey(c.Id, keyPart))
	}
	return c.s.resolveNotificationsForResourceKeys(ctx, "connection", c.Id, keys)
}
