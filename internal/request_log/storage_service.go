package request_log

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apblob"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util"
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
func (ss *StorageService) GetRecord(ctx context.Context, id uuid.UUID) (*LogRecord, error) {
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

func (ss *StorageService) GetFullLog(ctx context.Context, id uuid.UUID) (*FullLog, error) {
	log, err := ss.GetRecord(ctx, id)
	if err != nil {
		return nil, err
	}

	return ss.fullStore.GetFullLog(ctx, log.Namespace, id)
}

// Migrate runs any necessary schema migrations for the storage backend.
func (ss *StorageService) Migrate(ctx context.Context) error {
	ss.logger.Info("running request log migrations")
	defer ss.logger.Info("request log migrations complete")

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
func NewStorageService(
	ctx context.Context,
	cfg *config.HttpLogging,
	cursorKey config.KeyDataType,
	logger *slog.Logger,
) (*StorageService, error) {
	logger = logger.With("service", "request_log")
	store := NewRecordStore(cfg.Database, logger)
	retriever := NewRecordRetriever(cfg.Database, cursorKey, logger)
	blobStore, err := apblob.NewFromConfig(ctx, cfg.BlobStorage)
	if err != nil {
		return nil, err
	}
	fullStore := NewBlobStore(blobStore, logger)

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
