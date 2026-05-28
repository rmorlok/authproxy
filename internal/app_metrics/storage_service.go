package app_metrics

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/rmorlok/authproxy/internal/apblob"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type StorageService struct {
	logger        *slog.Logger
	store         RecordStore
	fullStore     FullStore
	retriever     RecordRetriever
	captureConfig captureConfig
}

func (ss *StorageService) NewRoundTripper(ri httpf.RequestInfo, transport http.RoundTripper) http.RoundTripper {
	return &RoundTripper{
		store:         ss.store,
		fullStore:     ss.fullStore,
		logger:        ss.logger,
		captureConfig: ss.captureConfig,
		requestInfo:   ri,
		transport:     transport,
	}
}

// GetRecord retrieves a single LogRecord by its request ID.
func (ss *StorageService) GetRecord(ctx context.Context, id apid.ID) (*LogRecord, error) {
	return ss.retriever.GetRecord(ctx, id)
}

// NewListRequestsBuilder creates a new builder for listing entry records with filters.
func (ss *StorageService) NewListRequestsBuilder() ListRequestBuilder {
	return ss.retriever.NewListRequestsBuilder()
}

// ListRequestsFromCursor resumes a paginated listing from a cursor string.
func (ss *StorageService) ListRequestsFromCursor(ctx context.Context, cursor string) (ListRequestExecutor, error) {
	return ss.retriever.ListRequestsFromCursor(ctx, cursor)
}

// QueryRequestEventMetrics executes time-series metric queries over request events.
func (ss *StorageService) QueryRequestEventMetrics(ctx context.Context, queries []RequestEventMetricsQuery) ([]RequestEventMetricSeries, error) {
	return ss.retriever.QueryRequestEventMetrics(ctx, queries)
}

func (ss *StorageService) StoreConnectionResourceSamples(ctx context.Context, samples []*ConnectionResourceSample) error {
	return ss.store.(ResourceSampleStore).StoreConnectionResourceSamples(ctx, samples)
}

func (ss *StorageService) StoreActorResourceSamples(ctx context.Context, samples []*ActorResourceSample) error {
	return ss.store.(ResourceSampleStore).StoreActorResourceSamples(ctx, samples)
}

func (ss *StorageService) ListConnectionResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ConnectionResourceSample, error) {
	return ss.retriever.(ResourceSampleRetriever).ListConnectionResourceSamples(ctx, query)
}

func (ss *StorageService) ListActorResourceSamples(ctx context.Context, query ResourceSampleQuery) ([]*ActorResourceSample, error) {
	return ss.retriever.(ResourceSampleRetriever).ListActorResourceSamples(ctx, query)
}

func (ss *StorageService) GetFullLog(ctx context.Context, id apid.ID) (*FullLog, error) {
	log, err := ss.GetRecord(ctx, id)
	if err != nil {
		return nil, err
	}

	return ss.fullStore.GetFullLog(ctx, log.Namespace, id)
}

// Ping checks if the storage backends are reachable.
func (ss *StorageService) Ping(ctx context.Context) bool {
	if p, ok := ss.store.(pingable); ok {
		if !p.Ping(ctx) {
			return false
		}
	}

	if util.SameInstance(ss.store, ss.fullStore) {
		return true
	}

	if p, ok := ss.fullStore.(pingable); ok {
		return p.Ping(ctx)
	}

	return true
}

// Migrate runs any necessary schema migrations for the storage backend.
func (ss *StorageService) Migrate(ctx context.Context) error {
	ss.logger.Info("running request events migrations")
	defer ss.logger.Info("request events migrations complete")

	if m, ok := ss.store.(migratable); ok {
		ss.logger.Info("running store migrations")
		if err := m.Migrate(ctx); err != nil {
			return err
		}
	}

	if util.SameInstance(ss.store, ss.fullStore) {
		return nil
	}

	if m, ok := ss.fullStore.(migratable); ok {
		ss.logger.Info("running full store migrations")
		return m.Migrate(ctx)
	}

	return nil
}

// NewStorageService that will store the log records and the full request/response.
// dbOpts are forwarded to the underlying DB constructors — pass
// database.WithTelemetry(...) to instrument the request-events database tier.
func NewStorageService(
	ctx context.Context,
	cfg *config.HttpLogging,
	cursorEncryptor pagination.CursorEncryptor,
	encryptor Encryptor,
	logger *slog.Logger,
	dbOpts ...database.Option,
) (*StorageService, error) {
	logger = logger.With("service", "app_metrics")
	store := NewRecordStore(cfg.Database, logger, dbOpts...)
	retriever := NewRecordRetriever(cfg.Database, cursorEncryptor, logger, dbOpts...)
	blobStore, err := apblob.NewFromConfig(ctx, cfg.BlobStorage)
	if err != nil {
		return nil, err
	}
	fullStore := NewBlobStore(blobStore, encryptor, logger)

	cc := captureConfig{}
	cc.setFromConfig(cfg)

	return &StorageService{
		store:         store,
		logger:        logger,
		retriever:     retriever,
		fullStore:     fullStore,
		captureConfig: cc,
	}, nil
}
