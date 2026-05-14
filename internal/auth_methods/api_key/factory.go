package api_key

import (
	"log/slog"

	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type factory struct {
	db      database.DB
	encrypt encrypt.E
	httpf   httpf.F
	logger  *slog.Logger
}

// NewFactory constructs an api-key connection factory. The factory is owned
// by the core service and shared across all api-key connections (one db /
// encrypt / httpf / logger dependency set per service).
func NewFactory(db database.DB, encrypt encrypt.E, httpf httpf.F, logger *slog.Logger) Factory {
	return &factory{
		db:      db,
		encrypt: encrypt,
		httpf:   httpf,
		logger:  logger,
	}
}

func (f *factory) NewApiKey(connection coreIface.Connection) ApiKeyConnection {
	return &apiKeyConnection{
		db:         f.db,
		encrypt:    f.encrypt,
		httpf:      f.httpf,
		logger:     f.logger,
		connection: connection,
	}
}
