package request_log

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/rmorlok/authproxy/internal/apblob"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/util"
)

// Encryptor provides namespace-scoped encryption/decryption for blob storage.
// Satisfied by encrypt.E via duck typing.
type Encryptor interface {
	EncryptForNamespace(ctx context.Context, namespacePath string, data []byte) (encfield.EncryptedField, error)
	Decrypt(ctx context.Context, ef encfield.EncryptedField) ([]byte, error)
}

type FullStore interface {
	// Store persists a FullLog to the storage backend.
	Store(ctx context.Context, log *FullLog) error

	// GetFullLog retrieves a FullLog from the storage backend.
	GetFullLog(ctx context.Context, ns string, id apid.ID) (*FullLog, error)
}

type blobStore struct {
	client    apblob.Client
	encryptor Encryptor
	logger    *slog.Logger
}

func NewBlobStore(client apblob.Client, encryptor Encryptor, logger *slog.Logger) FullStore {
	return &blobStore{
		client:    client,
		encryptor: encryptor,
		logger:    logger,
	}
}

func (s *blobStore) pathFor(ns string, id apid.ID) string {
	return ns + "/" + id.String() + ".enc"
}

func (s *blobStore) Store(ctx context.Context, log *FullLog) error {
	jsonData, err := json.Marshal(log)
	if err != nil {
		s.logger.Error("error serializing entry to JSON", "error", err, "record_id", log.Id.String())
		return nil
	}

	ef, err := s.encryptor.EncryptForNamespace(ctx, log.Namespace, jsonData)
	if err != nil {
		s.logger.Error("error encrypting full HTTP log entry", "error", err, "record_id", log.Id.String())
		return nil
	}

	if err := s.client.Put(
		ctx,
		apblob.PutInput{
			Key:         s.pathFor(log.Namespace, log.Id),
			Data:        []byte(ef.ToInlineString()),
			ContentType: util.ToPtr("application/octet-stream"),
		}); err != nil {
		s.logger.Error("error storing full HTTP log entry in blob storage", "error", err, "record_id", log.Id.String())
	}

	return nil
}

func (s *blobStore) GetFullLog(ctx context.Context, ns string, id apid.ID) (*FullLog, error) {
	data, err := s.client.Get(ctx, s.pathFor(ns, id))
	if err != nil {
		return nil, err
	}

	ef, err := encfield.ParseInlineString(string(data))
	if err != nil {
		return nil, err
	}

	plaintext, err := s.encryptor.Decrypt(ctx, ef)
	if err != nil {
		return nil, err
	}

	var log FullLog
	return &log, json.Unmarshal(plaintext, &log)
}
