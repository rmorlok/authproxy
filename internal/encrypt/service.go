package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

const globalScope = "global"
const memorySyncPeriod = 5 * time.Minute
const maxInitialWait = 5 * time.Minute

type service struct {
	cfg    config.C
	db     database.DB
	logger *slog.Logger

	mu              sync.RWMutex
	keyDataCache    map[string]*sconfig.KeyData
	keyCache        map[apid.ID][]byte // ekv_id → key bytes
	currentKeyCache map[string]apid.ID // scope → current ekv_id

	stopCh    chan struct{} // signal goroutine to stop
	doneCh    chan struct{} // closed when goroutine exits
	syncReady chan struct{} // closed after first successful sync
}

func NewTestEncryptService(
	cfg config.C,
	db database.DB,
) (config.C, E) {
	if cfg == nil {
		cfg = config.FromRoot(&sconfig.Root{})
	}

	if cfg.GetRoot().SystemAuth.GlobalAESKey == nil {
		cfg.GetRoot().SystemAuth.GlobalAESKey = &sconfig.KeyData{InnerVal: &sconfig.KeyDataRandomBytes{}}
	}

	svc := NewEncryptService(cfg, db, slog.Default())

	// For tests, sync keys to DB and memory synchronously without starting the goroutine.
	if s, ok := svc.(*service); ok {
		s.startForTest()
	}

	return cfg, svc
}

func NewEncryptService(
	cfg config.C,
	db database.DB,
	logger *slog.Logger,
) E {
	if cfg != nil && cfg.GetRoot().DevSettings.IsFakeEncryptionEnabled() {
		doBase64Encode := !cfg.GetRoot().DevSettings.IsFakeEncryptionSkipBase64Enabled()
		return NewFakeEncryptService(doBase64Encode)
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &service{
		cfg:             cfg,
		db:              db,
		logger:          logger,
		keyDataCache:    make(map[string]*sconfig.KeyData),
		keyCache:        make(map[apid.ID][]byte),
		currentKeyCache: make(map[string]apid.ID),
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
		syncReady:       make(chan struct{}),
	}
}

func (s *service) getKeyForScope(ctx context.Context, scope string) (*sconfig.KeyData, error) {
	s.mu.Lock()
	keyData, ok := s.keyDataCache[scope]
	s.mu.Unlock()

	if ok {
		return keyData, nil
	}

	if scope == globalScope {
		keyData = s.cfg.GetRoot().SystemAuth.GlobalAESKey
	}

	if keyData != nil {
		s.mu.Lock()
		s.keyDataCache[scope] = keyData
		s.mu.Unlock()

		return keyData, nil
	}

	return nil, fmt.Errorf("unknown scope %q", scope)
}

// SyncKeysFromDbToMemory reads all keys from the database and loads the data into memory, with mapping
// from scope to key id, as well as key id to the key for that data. This method is best-effort. It will
// return a multi error for all errors, but still swap the memory caches with the successfully loaded data.
func (s *service) syncKeysFromDbToMemory(ctx context.Context) error {
	if s == nil || s.cfg == nil || s.cfg.GetRoot() == nil {
		return errors.New("no configuration available")
	}

	// Re-use the existing cached data because given id can never change value,
	// but rather than making the cache permanent we transfer every time so that
	// any values that no longer exist in the database will be removed from memory
	var oldKeyCache map[apid.ID][]byte
	var newKeyCache map[apid.ID][]byte
	var newCurrentKeyCache map[string]apid.ID
	var merr *multierror.Error

	func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		// Reset so we re-pull key datas from database
		s.keyDataCache = make(map[string]*sconfig.KeyData, len(s.keyDataCache))

		oldKeyCache = make(map[apid.ID][]byte, len(s.keyCache))
		newKeyCache = make(map[apid.ID][]byte, len(s.keyCache))
		newCurrentKeyCache = make(map[string]apid.ID, len(s.currentKeyCache))

		for id, data := range s.keyCache {
			oldKeyCache[id] = data
		}
	}()

	_ = s.db.EnumerateEncryptionKeyVersions(
		ctx,
		func(ekvs []*database.EncryptionKeyVersion, lastPage bool) (stop bool, err error) {
			for _, ekv := range ekvs {
				if ekv.IsCurrent {
					newCurrentKeyCache[ekv.Scope] = ekv.Id
				}

				if data, ok := oldKeyCache[ekv.Id]; ok {
					// The data for a given version is immutable, so we can just take old value
					newKeyCache[ekv.Id] = data
				} else {
					// This is a new version, pull from the actual source
					keyData, err := s.getKeyForScope(ctx, ekv.Scope)
					if err != nil {
						merr = multierror.Append(merr, errors.Wrapf(err, "failed to get key for scope '%q' for key version id %q", ekv.Scope, ekv.Id))
						continue
					}

					kvi, err := keyData.GetVersion(ctx, ekv.ProviderVersion)
					if err != nil {
						merr = multierror.Append(merr, errors.Wrapf(err, "failed to get key version for scope '%q' for key version id %q", ekv.Scope, ekv.Id))
						continue
					}

					newKeyCache[ekv.Id] = kvi.Data
				}
			}

			return false, nil
		},
	)

	s.mu.Lock()
	s.keyCache = newKeyCache
	s.currentKeyCache = newCurrentKeyCache
	s.mu.Unlock()

	return merr.ErrorOrNil()
}

// ensureSynced blocks until the background goroutine has completed its first successful sync,
// or the context is cancelled.
func (s *service) ensureSynced(ctx context.Context) error {
	select {
	case <-s.syncReady:
		return nil
	default:
	}

	// Not ready yet, block until ready or context cancelled.
	select {
	case <-s.syncReady:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Start launches the background key sync goroutine.
func (s *service) Start() {
	go s.syncLoop()
}

// Shutdown stops the background key sync goroutine and waits for it to exit.
func (s *service) Shutdown() {
	close(s.stopCh)
	<-s.doneCh
}

// startForTest performs a single synchronous sync and marks the service as ready,
// without starting the background goroutine. Used by NewTestEncryptService.
func (s *service) startForTest() {
	ctx := context.Background()

	// Sync keys from config to database
	syncKeysToDatabase(ctx, s.cfg, s.db, s.logger, nil)

	// Sync keys from database to memory
	if err := s.syncKeysFromDbToMemory(ctx); err != nil {
		s.logger.Warn("test encrypt service: failed to sync keys from db to memory", "error", err)
	}

	close(s.syncReady)
	close(s.doneCh) // No goroutine to wait for
}

func (s *service) syncLoop() {
	defer close(s.doneCh)

	ctx := context.Background()
	deadline := time.Now().Add(maxInitialWait)

	// Initial sync with exponential backoff
	backoff := 1 * time.Second
	maxBackoff := 30 * time.Second

	for {
		if err := s.syncKeysFromDbToMemory(ctx); err != nil {
			s.logger.Warn("encrypt service: initial key sync failed", "error", err)
		}

		// Check if global AES key is available
		s.mu.RLock()
		_, hasGlobal := s.currentKeyCache[globalScope]
		s.mu.RUnlock()

		if hasGlobal {
			close(s.syncReady)
			break
		}

		if time.Now().After(deadline) {
			panic("encrypt service: failed to sync global AES key within 5 minutes")
		}

		s.logger.Info("encrypt service: global key not available yet, retrying", "backoff", backoff)

		select {
		case <-s.stopCh:
			return
		case <-time.After(backoff):
		}

		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	// Periodic sync every 5 minutes
	ticker := time.NewTicker(memorySyncPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if err := s.syncKeysFromDbToMemory(ctx); err != nil {
				s.logger.Warn("encrypt service: periodic key sync failed", "error", err)
			}
		}
	}
}

// getCurrentKeyID returns the current key ID for the given scope.
func (s *service) getCurrentKeyID(scope string) (apid.ID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.currentKeyCache[scope]
	if !ok {
		return apid.Nil, fmt.Errorf("no current key for scope %q", scope)
	}
	return id, nil
}

// getKeyBytes returns the key bytes for the given ekv_id, falling back to DB if not cached.
func (s *service) getKeyBytes(ctx context.Context, ekvID apid.ID) ([]byte, error) {
	s.mu.RLock()
	data, ok := s.keyCache[ekvID]
	s.mu.RUnlock()

	if ok {
		return data, nil
	}

	// Fallback: load from DB and then fetch from provider
	ekv, err := s.db.GetEncryptionKeyVersion(ctx, ekvID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get encryption key version %s from database", ekvID)
	}

	// We need to fetch the key data from the config-based providers
	kd, err := s.getKeyForScope(ctx, ekv.Scope)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get key for scope '%q' for key version id %q", ekv.Scope, ekvID)
	}

	ver, err := kd.GetCurrentVersion(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get current key version for scope '%q' for key version id %q", ekv.Scope, ekvID)
	}

	if string(ver.Provider) == ekv.Provider &&
		ver.ProviderID == ekv.ProviderID &&
		ver.ProviderVersion == ekv.ProviderVersion {
		s.mu.Lock()
		s.keyCache[ekvID] = ver.Data
		s.mu.Unlock()
		return ver.Data, nil
	}

	return nil, fmt.Errorf("could not find key data for encryption key version %s (provider=%s, id=%s, version=%s)",
		ekvID, ekv.Provider, ekv.ProviderID, ekv.ProviderVersion)
}

// getAllKeyBytes returns all cached key bytes for trying decryption.
func (s *service) getAllKeyBytes() [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([][]byte, 0, len(s.keyCache))
	for _, data := range s.keyCache {
		keys = append(keys, data)
	}
	return keys
}

func encryptWithKey(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AES cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create GCM")
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.Wrap(err, "failed to generate nonce")
	}

	encryptedData := gcm.Seal(nonce, nonce, data, nil)
	return encryptedData, nil
}

func decryptWithKey(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AES cipher")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create GCM")
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("data length is too short to contain nonce")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	decryptedData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "decryption failed")
	}

	return decryptedData, nil
}

// decryptWithAnyKey tries to decrypt data with each key in order, returning the first success.
func decryptWithAnyKey(keys [][]byte, data []byte) ([]byte, error) {
	var lastErr error
	for _, key := range keys {
		result, err := decryptWithKey(key, data)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, errors.Wrap(lastErr, "decryption failed with all keys")
}

// EncryptGlobal encrypts raw bytes with the current key.
// For raw byte methods, no version prefix is added (used for ephemeral data like session cookies).
func (s *service) EncryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return nil, err
	}

	keyID, err := s.getCurrentKeyID(globalScope)
	if err != nil {
		return nil, err
	}

	keyBytes, err := s.getKeyBytes(ctx, keyID)
	if err != nil {
		return nil, err
	}

	return encryptWithKey(keyBytes, data)
}

func (s *service) EncryptForConnection(ctx context.Context, connection Connection, data []byte) ([]byte, error) {
	return s.EncryptGlobal(ctx, data)
}

func (s *service) EncryptForConnector(ctx context.Context, connection ConnectorVersion, data []byte) ([]byte, error) {
	return s.EncryptGlobal(ctx, data)
}

// DecryptGlobal decrypts raw bytes by trying all keys.
// For raw byte methods, there is no version prefix (used for ephemeral data).
func (s *service) DecryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return nil, err
	}

	keys := s.getAllKeyBytes()
	return decryptWithAnyKey(keys, data)
}

func (s *service) DecryptForConnection(ctx context.Context, connection Connection, data []byte) ([]byte, error) {
	return s.DecryptGlobal(ctx, data)
}

func (s *service) DecryptForConnector(ctx context.Context, cv ConnectorVersion, data []byte) ([]byte, error) {
	return s.DecryptGlobal(ctx, data)
}

// EncryptStringGlobal encrypts a string with the current key and returns a versioned string.
// Output format: "<ekv_id>:<base64>"
func (s *service) EncryptStringGlobal(ctx context.Context, data string) (string, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return "", err
	}

	keyID, err := s.getCurrentKeyID(globalScope)
	if err != nil {
		return "", err
	}

	keyBytes, err := s.getKeyBytes(ctx, keyID)
	if err != nil {
		return "", err
	}

	encryptedData, err := encryptWithKey(keyBytes, []byte(data))
	if err != nil {
		return "", err
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return fmt.Sprintf("%s:%s", keyID, encodedData), nil
}

func (s *service) EncryptStringForConnection(ctx context.Context, connection Connection, data string) (string, error) {
	return s.EncryptStringGlobal(ctx, data)
}

func (s *service) EncryptStringForConnector(ctx context.Context, cv ConnectorVersion, data string) (string, error) {
	return s.EncryptStringGlobal(ctx, data)
}

// DecryptStringGlobal decrypts a string that may be in new format (<ekv_id>:<base64>)
// or legacy formats ("v1:<keyIndex>:<base64>" or "<base64>").
func (s *service) DecryptStringGlobal(ctx context.Context, base64Data string) (string, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return "", err
	}

	// New format: "<ekv_id>:<base64>" where ekv_id starts with "ekv_"
	if strings.HasPrefix(base64Data, string(apid.PrefixEncryptionKeyVersion)) {
		colonIdx := strings.Index(base64Data, ":")
		if colonIdx < 0 {
			return "", errors.New("invalid encrypted string: missing separator after ekv_id")
		}

		ekvIDStr := base64Data[:colonIdx]
		encodedData := base64Data[colonIdx+1:]

		ekvID := apid.ID(ekvIDStr)
		keyBytes, err := s.getKeyBytes(ctx, ekvID)
		if err != nil {
			return "", errors.Wrapf(err, "failed to get key for ekv_id %s", ekvID)
		}

		decodedData, err := base64.StdEncoding.DecodeString(encodedData)
		if err != nil {
			return "", errors.Wrap(err, "failed to decode base64 string")
		}

		decryptedData, err := decryptWithKey(keyBytes, decodedData)
		if err != nil {
			return "", err
		}

		return string(decryptedData), nil
	}

	// Legacy v1 format: "v1:<keyIndex>:<base64>" - try all keys
	if strings.HasPrefix(base64Data, "v1:") {
		rest := base64Data[3:]
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			return "", errors.New("invalid versioned encrypted string: missing key index separator")
		}

		encodedData := rest[colonIdx+1:]

		decodedData, err := base64.StdEncoding.DecodeString(encodedData)
		if err != nil {
			return "", errors.Wrap(err, "failed to decode base64 string")
		}

		keys := s.getAllKeyBytes()
		decryptedData, err := decryptWithAnyKey(keys, decodedData)
		if err != nil {
			return "", err
		}

		return string(decryptedData), nil
	}

	// Legacy format: no prefix, try all keys
	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode base64 string")
	}

	keys := s.getAllKeyBytes()
	decryptedData, err := decryptWithAnyKey(keys, decodedData)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}

func (s *service) DecryptStringForConnection(ctx context.Context, connection Connection, base64Data string) (string, error) {
	return s.DecryptStringGlobal(ctx, base64Data)
}

func (s *service) DecryptStringForConnector(ctx context.Context, cv ConnectorVersion, base64Data string) (string, error) {
	return s.DecryptStringGlobal(ctx, base64Data)
}

// IsEncryptedWithCurrentKey checks whether a string value was encrypted with the current key.
func (s *service) IsEncryptedWithCurrentKey(base64Str string) bool {
	s.mu.RLock()
	currentID, ok := s.currentKeyCache[globalScope]
	s.mu.RUnlock()

	if !ok {
		return false
	}

	return strings.HasPrefix(base64Str, string(currentID)+":")
}
