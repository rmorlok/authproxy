package oauth2

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/context"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/jwt"
	"github.com/rmorlok/authproxy/redis"
)

type Factory interface {
	NewOAuth2(connection database.Connection, connector config.Connector) *OAuth2
	GetOAuth2State(ctx context.Context, actor jwt.Actor, stateId uuid.UUID) (*OAuth2, error)
}

type factory struct {
	cfg     config.C
	db      database.DB
	redis   redis.R
	httpf   httpf.F
	encrypt encrypt.E
}

func NewFactory(cfg config.C, db database.DB, redis redis.R, httpf httpf.F, encrypt encrypt.E) Factory {
	return &factory{
		cfg:     cfg,
		db:      db,
		redis:   redis,
		httpf:   httpf,
		encrypt: encrypt,
	}
}

func (f *factory) NewOAuth2(connection database.Connection, connector config.Connector) *OAuth2 {
	return newOAuth2(
		f.cfg,
		f.db,
		f.redis,
		f.httpf,
		f.encrypt,
		connection,
		connector,
	)
}

func (f *factory) GetOAuth2State(ctx context.Context, actor jwt.Actor, stateId uuid.UUID) (*OAuth2, error) {
	return getOAuth2State(
		ctx,
		f.cfg,
		f.db,
		f.redis,
		f.httpf,
		f.encrypt,
		actor,
		stateId,
	)
}
