package api_key

import (
	"log/slog"

	coreIface "github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encrypt"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type apiKeyConnection struct {
	db         database.DB
	encrypt    encrypt.E
	httpf      httpf.F
	logger     *slog.Logger
	connection coreIface.Connection
}

var _ coreIface.Proxy = (*apiKeyConnection)(nil)
var _ ApiKeyConnection = (*apiKeyConnection)(nil)
