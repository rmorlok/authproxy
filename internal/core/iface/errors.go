package iface

import (
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/database"
)

var ErrNotFound = database.ErrNotFound
var ErrConnectionNotFound = errors.Wrap(ErrNotFound, "connection not found")
