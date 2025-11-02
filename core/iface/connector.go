package iface

import "github.com/rmorlok/authproxy/database"

type Connector interface {
	ConnectorVersion
	GetTotalVersions() int64
	GetStates() database.ConnectorVersionStates
}
