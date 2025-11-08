package iface

import "github.com/rmorlok/authproxy/internal/database"

type Connector interface {
	ConnectorVersion
	GetTotalVersions() int64
	GetStates() database.ConnectorVersionStates
}
