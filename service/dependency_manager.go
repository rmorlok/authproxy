package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/aplog"
	"github.com/rmorlok/authproxy/apredis"
	"github.com/rmorlok/authproxy/config"
	"github.com/rmorlok/authproxy/connectors"
	connectorsinterface "github.com/rmorlok/authproxy/connectors/interface"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/encrypt"
	"github.com/rmorlok/authproxy/httpf"
	"github.com/rmorlok/authproxy/request_log"
)

type DependencyManager struct {
	serviceId      string
	cfg            config.C
	logBuilder     aplog.Builder
	logger         *slog.Logger
	r              apredis.Client
	db             database.DB
	httpf          httpf.F
	logRetriever   request_log.LogRetriever
	e              encrypt.E
	asynqClient    *asynq.Client
	asynqInspector *asynq.Inspector
	c              connectorsinterface.C
}

func NewDependencyManager(serviceId string, cfg config.C) *DependencyManager {
	return &DependencyManager{
		serviceId: serviceId,
		cfg:       cfg,
	}
}

func (dm *DependencyManager) GetConfig() config.C {
	return dm.cfg
}

func (dm *DependencyManager) GetConfigRoot() *config.Root {
	return dm.cfg.GetRoot()
}

func (dm *DependencyManager) GetServiceId() string {
	return dm.serviceId
}

func (dm *DependencyManager) GetLogBuilder() aplog.Builder {
	if dm.logBuilder == nil {
		dm.logBuilder = aplog.NewBuilder(dm.cfg.GetRootLogger())
	}

	return dm.logBuilder
}

func (dm *DependencyManager) GetRootLogger() *slog.Logger {
	return dm.GetConfigRoot().GetRootLogger()
}

func (dm *DependencyManager) GetLogger() *slog.Logger {
	if dm.logger == nil {
		b := dm.GetLogBuilder()
		b = b.WithService(dm.serviceId)
		dm.logger = b.Build()
	}

	return dm.logger
}

func (dm *DependencyManager) GetRedisClient() apredis.Client {
	if dm.r == nil {
		var err error
		dm.r, err = apredis.NewForRoot(context.Background(), dm.GetConfig().GetRoot())
		if err != nil {
			panic(err)
		}
	}

	return dm.r
}

func (dm *DependencyManager) GetDatabase() database.DB {
	if dm.db == nil {
		var err error
		dm.db, err = database.NewConnectionForRoot(dm.GetConfigRoot(), dm.GetLogger())
		if err != nil {
			panic(err)
		}
	}

	return dm.db
}

// AutoMigrateDatabase will attempt to migrate the database if the root config has auto migrate enabled.
func (dm *DependencyManager) AutoMigrateDatabase() {
	if dm.GetConfigRoot().Database.GetAutoMigrate() {
		func() {
			m := apredis.NewMutex(
				dm.GetRedisClient(),
				database.MigrateMutexKeyName,
				apredis.MutexOptionLockFor(dm.GetConfigRoot().Database.GetAutoMigrationLockDuration()),
				apredis.MutexOptionRetryFor(dm.GetConfigRoot().Database.GetAutoMigrationLockDuration()+1*time.Second),
				apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				apredis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(errors.Wrap(err, "failed to establish lock for database migration"))
			}
			defer m.Unlock(context.Background())

			if err := dm.GetDatabase().Migrate(context.Background()); err != nil {
				panic(err)
			}
		}()
	}
}

func (dm *DependencyManager) GetHttpf() httpf.F {
	if dm.httpf == nil {
		dm.httpf = httpf.CreateFactory(dm.GetConfig(), dm.GetRedisClient(), dm.GetLogger())
	}

	return dm.httpf
}

func (dm *DependencyManager) GetRequestLogRetriever() request_log.LogRetriever {
	if dm.logRetriever == nil {
		dm.logRetriever = request_log.NewRetrievalService(dm.GetRedisClient(), dm.GetConfig().GetGlobalKey())
	}

	return dm.logRetriever
}

func (dm *DependencyManager) AutoMigrateLogRetriever() {
	if dm.GetConfigRoot().HttpLogging.GetAutoMigrate() {
		err := request_log.Migrate(context.Background(), dm.GetRedisClient(), dm.GetLogger())
		if err != nil {
			panic(err)
		}
	}
}

func (dm *DependencyManager) GetEncryptService() encrypt.E {
	if dm.e == nil {
		dm.e = encrypt.NewEncryptService(dm.GetConfig(), dm.GetDatabase())
	}

	return dm.e
}

func (dm *DependencyManager) GetAsyncClient() *asynq.Client {
	if dm.asynqClient == nil {
		dm.asynqClient = asynq.NewClientFromRedisClient(dm.GetRedisClient())
	}

	return dm.asynqClient
}

func (dm *DependencyManager) GetAsyncInspector() *asynq.Inspector {
	if dm.asynqInspector == nil {
		dm.asynqInspector = asynq.NewInspectorFromRedisClient(dm.GetRedisClient())
	}

	return dm.asynqInspector
}

func (dm *DependencyManager) GetConnectorsService() connectorsinterface.C {
	if dm.c == nil {
		dm.c = connectors.NewConnectorsService(
			dm.GetConfig(),
			dm.GetDatabase(),
			dm.GetEncryptService(),
			dm.GetRedisClient(),
			dm.GetHttpf(),
			dm.GetAsyncClient(),
			dm.GetLogger(),
		)
	}

	return dm.c
}

func (dm *DependencyManager) AutoMigrateConnectors() {
	if dm.GetConfigRoot().Connectors.GetAutoMigrate() {
		func() {
			m := apredis.NewMutex(
				dm.GetRedisClient(),
				connectors.MigrateMutexKeyName,
				apredis.MutexOptionLockFor(dm.GetConfigRoot().Connectors.GetAutoMigrationLockDurationOrDefault()),
				apredis.MutexOptionRetryFor(dm.GetConfigRoot().Connectors.GetAutoMigrationLockDurationOrDefault()+1*time.Second),
				apredis.MutexOptionRetryExponentialBackoff(100*time.Millisecond, 5*time.Second),
				apredis.MutexOptionDetailedLockMetadata(),
			)
			err := m.Lock(context.Background())
			if err != nil {
				panic(err)
			}
			defer m.Unlock(context.Background())

			if err := dm.GetConnectorsService().MigrateConnectors(context.Background()); err != nil {
				panic(err)
			}
		}()
	}
}

func (dm *DependencyManager) AutoMigrateAll() {
	dm.AutoMigrateDatabase()
	dm.AutoMigrateLogRetriever()
	dm.AutoMigrateConnectors()
}
