package request_log

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apblob"
	"github.com/rmorlok/authproxy/internal/util"
)

type FullStore interface {
	// Store persists a FullLog to the storage backend.
	Store(ctx context.Context, log *FullLog) error

	// GetFullLog retrieves a FullLog from the storage backend.
	GetFullLog(ctx context.Context, ns string, id uuid.UUID) (*FullLog, error)
}

type blobStore struct {
	client apblob.Client
	logger *slog.Logger
}

func NewBlobStore(client apblob.Client, logger *slog.Logger) FullStore {
	return &blobStore{
		client: client,
		logger: logger,
	}
}

func (s *blobStore) pathFor(ns string, id uuid.UUID) string {
	return ns + "/" + id.String() + ".json"
}

func (s *blobStore) Store(ctx context.Context, log *FullLog) error {
	jsonData, err := json.Marshal(log)
	if err != nil {
		s.logger.Error("error serializing entry to JSON", "error", err, "record_id", log.Id.String())
	} else {
		if err := s.client.Put(
			ctx,
			apblob.PutInput{
				Key:         s.pathFor(log.Namespace, log.Id),
				Data:        jsonData,
				ContentType: util.ToPtr("application/json"),
			}); err != nil {
			s.logger.Error("error storing full HTTP log entry in blob storage", "error", err, "record_id", log.Id.String())
		}
	}

	return nil
}

func (s *blobStore) GetFullLog(ctx context.Context, ns string, id uuid.UUID) (*FullLog, error) {
	data, err := s.client.Get(ctx, s.pathFor(ns, id))
	if err != nil {
		return nil, err
	}

	var log FullLog
	return &log, json.Unmarshal(data, &log)
}
