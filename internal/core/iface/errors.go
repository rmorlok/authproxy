package iface

import (
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
)

var ErrNotFound = database.ErrNotFound
var ErrConnectionNotFound = fmt.Errorf("connection not found: %w", ErrNotFound)
var ErrProtected = database.ErrProtected
var ErrDraftAlreadyExists = errors.New("a draft version already exists")
var ErrNotDraft = errors.New("version is not a draft")
