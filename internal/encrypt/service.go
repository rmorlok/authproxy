package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const globalScope = "global"
const memorySyncPeriod = 5 * time.Minute
const maxInitialWait = 5 * time.Minute

var globalEncryptionKeyID = database.GlobalEncryptionKeyID

type service struct {
	cfg    config.C
	db     database.DB
	logger *slog.Logger

	mu                         sync.RWMutex
	ekToKeyDataCache           map[apid.ID]*sconfig.KeyData        // encryption_key_id → key data config
	ekvToVersionInfoCache      map[apid.ID]*sconfig.KeyVersionInfo // ekv_id → key data for version
	ekToEkvCurrentVersionCache map[apid.ID]apid.ID                 // ek → ekv (key to current version)
	namespaceToEkCache         map[string]apid.ID                  // ek → ekv (key to current version)

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
		cfg:                        cfg,
		db:                         db,
		logger:                     logger,
		ekToKeyDataCache:           make(map[apid.ID]*sconfig.KeyData),
		ekvToVersionInfoCache:      make(map[apid.ID]*sconfig.KeyVersionInfo),
		ekToEkvCurrentVersionCache: make(map[apid.ID]apid.ID),
		namespaceToEkCache:         make(map[string]apid.ID),
		stopCh:                     make(chan struct{}),
		doneCh:                     make(chan struct{}),
		syncReady:                  make(chan struct{}),
	}
}

// syncKeysFromDbToMemory reads all keys from the database and loads the data into memory, with mapping
// from scope to key id, as well as key id to the key for that data. This method is best-effort. It will
// return a multi error for all errors, but still swap the memory caches with the successfully loaded data.
func (s *service) syncKeysFromDbToMemory(ctx context.Context) error {
	if s == nil || s.cfg == nil || s.cfg.GetRoot() == nil {
		return errors.New("no configuration available")
	}

	// Re-use the existing cached data because given id can never change value,
	// but rather than making the cache permanent we transfer every time so that
	// any values that no longer exist in the database will be removed from memory
	var newEkToKeyDataCache map[apid.ID]*sconfig.KeyData
	var oldEkvToVersionInfoCache map[apid.ID]*sconfig.KeyVersionInfo
	var newEkvToVersionInfoCache map[apid.ID]*sconfig.KeyVersionInfo
	var newEkToEkvCurrentVersionCache map[apid.ID]apid.ID
	var newNamespaceToEkCache map[string]apid.ID

	var merr *multierror.Error

	func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		newEkToKeyDataCache = make(map[apid.ID]*sconfig.KeyData, len(s.ekToKeyDataCache))
		newEkvToVersionInfoCache = make(map[apid.ID]*sconfig.KeyVersionInfo, len(s.ekvToVersionInfoCache))
		newEkToEkvCurrentVersionCache = make(map[apid.ID]apid.ID, len(s.ekToEkvCurrentVersionCache))
		newNamespaceToEkCache = make(map[string]apid.ID, len(s.namespaceToEkCache))
		oldEkvToVersionInfoCache = make(map[apid.ID]*sconfig.KeyVersionInfo, len(s.ekvToVersionInfoCache))

		for id, data := range s.ekvToVersionInfoCache {
			oldEkvToVersionInfoCache[id] = data
		}
	}()

	_, err := s.db.EnumerateEncryptionKeysInDependencyOrder(ctx, func(keys []*database.EncryptionKey, _ int) (stop bool, err error) {
		for _, key := range keys {
			var keyData *sconfig.KeyData
			if key.Id == globalEncryptionKeyID {
				// Value in database would be nil if this is the global key.
				keyData = s.cfg.GetRoot().SystemAuth.GlobalAESKey
			} else if key.EncryptedKeyData == nil || key.EncryptedKeyData.IsZero() {
				merr = multierror.Append(merr, fmt.Errorf("invalid encryption key data for key %q: %w", key.Id, err))
				continue
			} else {
				// Because we are enumerating in dependency order, the parent key should have already been loaded.
				// We can look up the ekv from the cache that has been stored to get the data to decrypt this key.
				ekv, ok := newEkvToVersionInfoCache[key.EncryptedKeyData.ID]
				if !ok {
					merr = multierror.Append(merr, fmt.Errorf("encryption key version %s not found in cache for key %q: %w", key.EncryptedKeyData.ID, key.Id, err))
					continue
				}

				keyDataBytes, err := decryptFieldWithBytes(ekv.Data, *key.EncryptedKeyData)
				if err != nil {
					merr = multierror.Append(merr, fmt.Errorf("failed to decrypt encryption key %q data for with ekv %q: %w", key.Id, key.EncryptedKeyData.ID, err))
					continue
				}

				var kd sconfig.KeyData
				if err := json.Unmarshal(keyDataBytes, &kd); err != nil {
					merr = multierror.Append(merr, fmt.Errorf("failed to unmarshal encryption key data for key %q: %w", key.Id, err))
					continue
				}

				keyData = &kd
			}

			// Cache the result.
			newEkToKeyDataCache[key.Id] = keyData

			// Now that we have the key data that can be used to pull the verion info for the key, load that version
			// info into cache, using the database defined identifiers for those versions. Any version not enumerated
			// in the database will be ignored as it is the database sync that defines when new versions become
			// available.
			_ = s.db.EnumerateEncryptionKeyVersionsForKey(ctx, key.Id,
				func(ekvs []*database.EncryptionKeyVersion, lastPage bool) (stop bool, err error) {
					for _, ekv := range ekvs {
						if ekv.IsCurrent {
							newEkToEkvCurrentVersionCache[ekv.EncryptionKeyId] = ekv.Id
						}

						if vi, ok := oldEkvToVersionInfoCache[ekv.Id]; ok {
							// The data for a given version is immutable, so we can just take old value
							newEkvToVersionInfoCache[ekv.Id] = vi
						} else {
							kvi, err := keyData.GetVersion(ctx, ekv.ProviderVersion)
							if err != nil {
								merr = multierror.Append(merr, fmt.Errorf("failed to get key version for encryption key %q for key version id %q: %w", ekv.EncryptionKeyId, ekv.Id, err))
								continue
							}

							newEkvToVersionInfoCache[ekv.Id] = &kvi
						}
					}

					return false, nil
				},
			)
		}

		return false, nil
	})

	if err != nil {
		merr = multierror.Append(merr, err)
	}

	// Identify all the keys used for the namespaces
	err = s.db.ListNamespacesBuilder().Enumerate(ctx, func(pr pagination.PageResult[database.Namespace]) (keepGoing bool, err error) {
		for _, ns := range pr.Results {
			if ns.EncryptionKeyId != nil {
				newNamespaceToEkCache[ns.Path] = *ns.EncryptionKeyId
			}
		}

		return true, nil
	})

	if err != nil {
		merr = multierror.Append(merr, err)
	}

	s.mu.Lock()
	s.ekvToVersionInfoCache = newEkvToVersionInfoCache
	s.ekToEkvCurrentVersionCache = newEkToEkvCurrentVersionCache
	s.namespaceToEkCache = newNamespaceToEkCache
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
	syncKeysVersionsToDatabase(ctx, s.cfg, s.db, s.logger, nil)

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

		hasGlobalVersion := false

		// Check if global AES key is available
		s.mu.RLock()
		if ekv, hasGlobal := s.ekToEkvCurrentVersionCache[globalEncryptionKeyID]; hasGlobal {
			_, hasGlobalVersion = s.ekvToVersionInfoCache[ekv]
		}
		s.mu.RUnlock()

		if hasGlobalVersion {
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

// getKeyIdForNamespace returns the ek_ for the given namespace. If that namespace is not configured it falls
// back to parent namespaces until reaching the global encryption key.
func (s *service) getKeyIdForNamespace(namespacePath string) (apid.ID, error) {
	paths := aschema.SplitNamespacePathToPrefixes(namespacePath)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := len(paths) - 1; i >= 0; i-- {
		if id, ok := s.namespaceToEkCache[paths[i]]; ok {
			return id, nil
		}
	}

	return globalEncryptionKeyID, nil
}

// getCurrentEkvId return the current ekv for the given ek
func (s *service) getCurrentEkvId(ekvId apid.ID) (apid.ID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	kevId, ok := s.ekToEkvCurrentVersionCache[ekvId]
	if !ok {
		return "", fmt.Errorf("no current key for encryption key %s", ekvId)
	}

	return kevId, nil
}

// getKeyBytes returns the key bytes for the given ekv_id, falling back to DB if not cached.
func (s *service) getKeyVersionBytes(ekvID apid.ID) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	vi, ok := s.ekvToVersionInfoCache[ekvID]

	if ok {
		return vi.Data, nil
	}

	return nil, fmt.Errorf("key version %s not found in cache", ekvID)
}

// getAllKeyBytes returns all cached key bytes for trying decryption.
func (s *service) getAllKeyBytes() [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([][]byte, 0, len(s.ekvToVersionInfoCache))
	for _, vi := range s.ekvToVersionInfoCache {
		keys = append(keys, vi.Data)
	}
	return keys
}

func encryptWithKey(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	encryptedData := gcm.Seal(nonce, nonce, data, nil)
	return encryptedData, nil
}

func decryptWithKey(key []byte, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("data length is too short to contain nonce")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	decryptedData, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
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
	return nil, fmt.Errorf("decryption failed with all keys: %w", lastErr)
}

// EncryptForKey encrypts data with the current version of the specified key. The caller is assumed to
// have validated that the cache sync has completed.
func (s *service) encryptForKey(ekId apid.ID, data []byte) (encfield.EncryptedField, error) {
	ekvId, err := s.getCurrentEkvId(ekId)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	keyBytes, err := s.getKeyVersionBytes(ekvId)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	encryptedData, err := encryptWithKey(keyBytes, data)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return encfield.EncryptedField{ID: ekvId, Data: encodedData}, nil
}

// EncryptForKey encrypts data with the current version of the specified key.
func (s *service) EncryptForKey(ctx context.Context, ekId apid.ID, data []byte) (encfield.EncryptedField, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return encfield.EncryptedField{}, err
	}
	return s.encryptForKey(ekId, data)
}

// EncryptStringForKey encrypts a string with the current version of the specified key.
func (s *service) EncryptStringForKey(ctx context.Context, ekId apid.ID, data string) (encfield.EncryptedField, error) {
	return s.EncryptForKey(ctx, ekId, []byte(data))
}

// EncryptGlobal encrypts raw bytes with the current global key.
func (s *service) EncryptGlobal(ctx context.Context, data []byte) (encfield.EncryptedField, error) {
	return s.EncryptForKey(ctx, globalEncryptionKeyID, data)
}

// EncryptStringGlobal encrypts a string with the current global key.
func (s *service) EncryptStringGlobal(ctx context.Context, data string) (encfield.EncryptedField, error) {
	return s.EncryptGlobal(ctx, []byte(data))
}

func (s *service) encryptForNamespace(namespacePath string, data []byte) (encfield.EncryptedField, error) {
	ekId, err := s.getKeyIdForNamespace(namespacePath)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	return s.encryptForKey(ekId, data)
}

func (s *service) EncryptForNamespace(ctx context.Context, namespacePath string, data []byte) (encfield.EncryptedField, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return encfield.EncryptedField{}, err
	}
	return s.encryptForNamespace(namespacePath, data)
}

func (s *service) EncryptStringForNamespace(ctx context.Context, namespacePath string, data string) (encfield.EncryptedField, error) {
	return s.EncryptForNamespace(ctx, namespacePath, []byte(data))
}

func (s *service) EncryptForEntity(ctx context.Context, entity NamespacedEntity, data []byte) (encfield.EncryptedField, error) {
	return s.EncryptForNamespace(ctx, entity.GetNamespace(), data)
}

func (s *service) EncryptStringForEntity(ctx context.Context, entity NamespacedEntity, data string) (encfield.EncryptedField, error) {
	return s.EncryptForEntity(ctx, entity, []byte(data))
}

func decryptFieldWithBytes(keyBytes []byte, ef encfield.EncryptedField) ([]byte, error) {
	decodedData, err := base64.StdEncoding.DecodeString(ef.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 string: %w", err)
	}

	decryptedData, err := decryptWithKey(keyBytes, decodedData)
	if err != nil {
		return nil, err
	}

	return decryptedData, nil
}

// Decrypt decrypts an EncryptedField using the key ID embedded in the field. It assumes that the cache
// sync has completed.
func (s *service) decrypt(ef encfield.EncryptedField) ([]byte, error) {
	if ef.IsZero() {
		return nil, nil
	}

	keyBytes, err := s.getKeyVersionBytes(ef.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get key for ekv_id %s: %w", ef.ID, err)
	}

	return decryptFieldWithBytes(keyBytes, ef)
}

// Decrypt decrypts an EncryptedField using the key ID embedded in the field.
func (s *service) Decrypt(ctx context.Context, ef encfield.EncryptedField) ([]byte, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return nil, err
	}

	return s.decrypt(ef)
}

// ReEncryptField decrypts the given encrypted field and re-encrypts it with the specified
// target key version. If the field is already encrypted with the target version, it is
// returned unchanged.
func (s *service) ReEncryptField(ctx context.Context, ef encfield.EncryptedField, targetEkvId apid.ID) (encfield.EncryptedField, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return encfield.EncryptedField{}, err
	}

	if ef.ID == targetEkvId {
		return ef, nil
	}

	plaintext, err := s.decrypt(ef)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	keyBytes, err := s.getKeyVersionBytes(targetEkvId)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	encrypted, err := encryptWithKey(keyBytes, plaintext)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	return encfield.EncryptedField{ID: targetEkvId, Data: base64.StdEncoding.EncodeToString(encrypted)}, nil
}

// SyncKeysFromDbToMemory forces a refresh of the in-memory key caches from the database.
func (s *service) SyncKeysFromDbToMemory(ctx context.Context) error {
	return s.syncKeysFromDbToMemory(ctx)
}

// DecryptString decrypts an EncryptedField using the key ID embedded in the field.
func (s *service) DecryptString(ctx context.Context, ef encfield.EncryptedField) (string, error) {
	decryptedData, err := s.Decrypt(ctx, ef)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}
