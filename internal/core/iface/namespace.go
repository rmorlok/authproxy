package iface

import (
	"time"

	"github.com/rmorlok/authproxy/internal/database"
)

type Namespace interface {
	GetPath() string
	GetState() database.NamespaceState
	GetCreatedAt() time.Time
	GetUpdatedAt() time.Time
}
