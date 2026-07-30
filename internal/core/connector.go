package core

import (
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
)

// Connector represents a logical connector together with the definition
// version selected for display.
type Connector struct {
	ConnectorVersion
}

func (c *Connector) GetNamespace() string {
	return c.Namespace
}

var _ iface.Connector = (*Connector)(nil)
var _ aplog.HasLogger = (*Connector)(nil)
