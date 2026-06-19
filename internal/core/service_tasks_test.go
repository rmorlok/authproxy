package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCronTasks_SchedulesOnlyEnabledPeriodicProbes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svc, db, _, _, _, encrypt := FullMockService(t, ctrl)
	connectionId := apid.New(apid.PrefixConnection)
	connector := &cschema.Connector{
		Id:          apid.New(apid.PrefixConnectorVersion),
		Version:     1,
		DisplayName: "periodic-conditional",
		Auth:        &cschema.Auth{InnerVal: &cschema.AuthApiKey{Type: cschema.AuthTypeAPIKey}},
		Probes: []cschema.Probe{
			{
				Id:     "enabled",
				Period: common.HumanDurationFor("1m"),
				Http:   &cschema.ProbeHttp{Method: "GET", URL: "https://x.example/health"},
			},
			{
				Id:     "disabled",
				Period: common.HumanDurationFor("2m"),
				If:     &common.Predicate{Javascript: `cfg.run_disabled === true`},
				Http:   &cschema.ProbeHttp{Method: "GET", URL: "https://y.example/health"},
			},
			{
				Id:   "manual",
				Http: &cschema.ProbeHttp{Method: "GET", URL: "https://z.example/health"},
			},
		},
	}
	conn := database.Connection{
		Id:               connectionId,
		Namespace:        "root",
		State:            database.ConnectionStateConfigured,
		HealthState:      database.ConnectionHealthStateHealthy,
		ConnectorId:      connector.Id,
		ConnectorVersion: connector.Version,
	}
	encryptedDef := encfield.EncryptedField{ID: "ekv_mock", Data: "encrypted-def"}
	connJSON, err := json.Marshal(connector)
	require.NoError(t, err)

	db.EXPECT().
		ListConnectionsBuilder().
		Return(&staticListConnectionsBuilder{connections: []database.Connection{conn}})
	db.EXPECT().
		GetConnectorVersion(gomock.Any(), connector.Id, connector.Version).
		Return(&database.ConnectorVersion{
			Id:                  connector.Id,
			Version:             connector.Version,
			State:               database.ConnectorVersionStatePrimary,
			Hash:                "hash",
			EncryptedDefinition: encryptedDef,
		}, nil)
	encrypt.EXPECT().
		DecryptString(gomock.Any(), encryptedDef).
		Return(string(connJSON), nil)

	tasks := svc.GetCronTasks()
	require.Len(t, tasks, 2)
	assert.Equal(t, taskTypeProbe, tasks[0].Task.Type())
	assert.Equal(t, "@every 1m0s", tasks[0].Cronspec)
	assert.Equal(t, taskTypeProbeOutcomeCleanup, tasks[1].Task.Type())
}

type staticListConnectionsBuilder struct {
	connections []database.Connection
}

func (b *staticListConnectionsBuilder) FetchPage(ctx context.Context) pagination.PageResult[database.Connection] {
	return pagination.PageResult[database.Connection]{Results: b.connections}
}

func (b *staticListConnectionsBuilder) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[database.Connection]) error {
	_, err := callback(b.FetchPage(ctx))
	return err
}

func (b *staticListConnectionsBuilder) Limit(int32) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) ForState(database.ConnectionState) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) ForStates([]database.ConnectionState) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) ForConnectorId(apid.ID) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) ForNamespaceMatcher(string) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) ForNamespaceMatchers([]string) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) OrderBy(database.ConnectionOrderByField, pagination.OrderBy) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) IncludeDeleted() database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) WithDeletedHandling(database.DeletedHandling) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) ForLabelSelector(string) database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) WithSetupStepNotNull() database.ListConnectionsBuilder {
	return b
}

func (b *staticListConnectionsBuilder) UpdatedBefore(time.Time) database.ListConnectionsBuilder {
	return b
}
