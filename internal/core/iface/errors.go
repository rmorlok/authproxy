package iface

import (
	stderrors "errors"

	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/database"
)

var ErrNotFound = database.ErrNotFound
var ErrConnectionNotFound = errors.Wrap(ErrNotFound, "connection not found")
var ErrDraftAlreadyExists = stderrors.New("a draft version already exists")
var ErrNotDraft = stderrors.New("version is not a draft")
