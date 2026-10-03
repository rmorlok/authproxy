package oauth2

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	dbmock "github.com/rmorlok/authproxy/internal/database/mock"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

type refreshCore struct {
	iface.C
	builder *refreshGenerations
}

func (c refreshCore) ListConnectorGenerationsBuilder() iface.ListConnectorGenerationsBuilder {
	return c.builder
}

type refreshGenerations struct {
	iface.ListConnectorGenerationsBuilder
	states     []database.ConnectorGenerationState
	connectors []iface.Connector
	err        error
}

func (b *refreshGenerations) ForStates(states []database.ConnectorGenerationState) iface.ListConnectorGenerationsBuilder {
	b.states = states
	return b
}
func (b *refreshGenerations) Enumerate(ctx context.Context, cb pagination.EnumerateCallback[iface.Connector]) error {
	if b.err != nil {
		return b.err
	}
	_, err := cb(pagination.PageResult[iface.Connector]{Results: b.connectors})
	return err
}

type refreshConnector struct {
	iface.Connector
	definition *cschema.ConnectorDefinition
}

func (c refreshConnector) GetDefinition() *cschema.ConnectorDefinition { return c.definition }

func TestRefreshScanUsesStoredConnectorOverrides(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "stored override", true: "read failure"}[fail], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := dbmock.NewMockDB(ctrl)
			builder := &refreshGenerations{connectors: []iface.Connector{
				refreshConnector{definition: &cschema.ConnectorDefinition{}},
				refreshConnector{
					definition: &cschema.ConnectorDefinition{
						Auth: &cschema.Auth{
							InnerVal: &cschema.AuthOAuth2{
								Token: cschema.AuthOauth2Token{
									RefreshTimeBeforeExpiry: &sconfig.HumanDuration{
										Duration: 45 * time.Minute,
									},
								},
							},
						},
					},
				},
			}}
			if fail {
				builder.err = errors.New("cannot read stored generations")
			} else {
				db.EXPECT().EnumerateOAuth2TokensExpiringWithin(gomock.Any(), 45*time.Minute, gomock.Any()).Return(nil)
			}

			handler := taskHandler{cfg: config.FromRoot(&sconfig.Root{}), db: db, core: refreshCore{builder: builder}, logger: aplog.NewNoopLogger()}
			task, err := newRefreshExpiringOauth2TokensTask()
			require.NoError(t, err)

			err = handler.refreshExpiringOauth2Tokens(context.Background(), task)
			if fail {
				require.ErrorIs(t, err, builder.err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, []database.ConnectorGenerationState{
				database.ConnectorGenerationStatePrimary,
				database.ConnectorGenerationStateActive,
			}, builder.states)
		})
	}
}
