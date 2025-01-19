package oauth2

import (
	"fmt"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/redis"
)

type OAuth2 struct {
	cfg        config.C
	db         database.DB
	redis      redis.R
	connection database.Connection
	connector  *config.Connector
	auth       *config.AuthOAuth2
	httpf      httpf.F
	encrypt    encrypt.E
	state      *state
}

func newOAuth2(
	cfg config.C,
	db database.DB,
	redis redis.R,
	httpf httpf.F,
	encrypt encrypt.E,
	connection database.Connection,
	connector config.Connector,
) *OAuth2 {
	auth, ok := connector.Auth.(*config.AuthOAuth2)
	if !ok {
		panic(fmt.Sprintf("connector id %s is not an oauth2 connector", connector.Id))
	}

	return &OAuth2{
		cfg:        cfg,
		db:         db,
		redis:      redis,
		connection: connection,
		auth:       auth,
		httpf:      httpf,
		encrypt:    encrypt,
		connector:  &connector,
	}
}
