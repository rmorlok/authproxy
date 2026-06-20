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
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

const globalScope = "global"
const memorySyncPeriod = 5 * time.Minute
const maxInitialWait = 5 * time.Minute

var globalEncryptionKeyID = database.GlobalKeyID

type service struct {
	cfg    config.C
	db     database.DB
	logger *slog.Logger

	mu                   sync.RWMutex
	keyToKeyDataCache    map[apid.ID]*sconfig.KeyData // key_id → key data config
	dekToBytesCache      map[apid.ID][]byte           // dek_id → plaintext DEK bytes
	keyToCurrentDEKCache map[apid.ID]apid.ID          // key_id → current dek_id
	namespaceToKeyCache  map[string]apid.ID           // namespace path → key_id

	stopCh    chan struct{} // signal goroutine to stop
	doneCh    chan struct{} // closed when goroutine exits
	syncReady chan struct{} // closed after first successful sync

	syncReadyOnce sync.Once
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
		cfg:                  cfg,
		db:                   db,
		logger:               logger,
		keyToKeyDataCache:    make(map[apid.ID]*sconfig.KeyData),
		dekToBytesCache:      make(map[apid.ID][]byte),
		keyToCurrentDEKCache: make(map[apid.ID]apid.ID),
		namespaceToKeyCache:  make(map[string]apid.ID),
		stopCh:               make(chan struct{}),
		doneCh:               make(chan struct{}),
		syncReady:            make(chan struct{}),
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
	var newKeyToKeyDataCache map[apid.ID]*sconfig.KeyData
	var oldDekToBytesCache map[apid.ID][]byte
	var newDekToBytesCache map[apid.ID][]byte
	var newKeyToCurrentDEKCache map[apid.ID]apid.ID
	var newNamespaceToKeyCache map[string]apid.ID

	var merr *multierror.Error

	func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		newKeyToKeyDataCache = make(map[apid.ID]*sconfig.KeyData, len(s.keyToKeyDataCache))
		newDekToBytesCache = make(map[apid.ID][]byte, len(s.dekToBytesCache))
		newKeyToCurrentDEKCache = make(map[apid.ID]apid.ID, len(s.keyToCurrentDEKCache))
		newNamespaceToKeyCache = make(map[string]apid.ID, len(s.namespaceToKeyCache))
		oldDekToBytesCache = make(map[apid.ID][]byte, len(s.dekToBytesCache))

		for id, data := range s.dekToBytesCache {
			oldDekToBytesCache[id] = append([]byte(nil), data...)
		}
	}()

	_, err := s.db.EnumerateKeysInDependencyOrder(ctx, func(keys []*database.Key, _ int) (keepGoing pagination.KeepGoing, err error) {
		for _, key := range keys {
			var keyData *sconfig.KeyData
			if key.Id == globalEncryptionKeyID {
				// Value in database would be nil if this is the global key.
				keyData = s.cfg.GetRoot().SystemAuth.GlobalAESKey
			} else if key.EncryptedKeyData == nil || key.EncryptedKeyData.IsZero() {
				merr = multierror.Append(merr, fmt.Errorf("invalid encryption key data for key %q: %w", key.Id, err))
				continue
			} else {
				// Because we are enumerating in dependency order, the parent key
				// should have already loaded the DEK that encrypted this child key data.
				keyBytes, ok := newDekToBytesCache[key.EncryptedKeyData.ID]
				if !ok {
					merr = multierror.Append(merr, fmt.Errorf("key material %s not found in cache for key %q: %w", key.EncryptedKeyData.ID, key.Id, err))
					continue
				}

				keyDataBytes, err := decryptFieldWithBytes(keyBytes, *key.EncryptedKeyData)
				if err != nil {
					merr = multierror.Append(merr, fmt.Errorf("failed to decrypt encryption key %q data with key material %q: %w", key.Id, key.EncryptedKeyData.ID, err))
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
			newKeyToKeyDataCache[key.Id] = keyData

			err = s.db.EnumerateDataEncryptionKeysForKey(ctx, key.Id,
				func(deks []*database.DataEncryptionKey, lastPage bool) (keepGoing pagination.KeepGoing, err error) {
					for _, dek := range deks {
						dekBytes, ok := oldDekToBytesCache[dek.Id]
						if !ok {
							infos := dataEncryptionKeyInfos([]*database.DataEncryptionKey{dek})
							if len(infos) != 1 {
								merr = multierror.Append(merr, fmt.Errorf("failed to map data encryption key %q for key %q", dek.Id, key.Id))
								continue
							}

							dekBytes, err = keyData.UnwrapDataEncryptionKey(ctx, infos[0])
							if err != nil {
								merr = multierror.Append(merr, fmt.Errorf("failed to unwrap data encryption key %q for key %q: %w", dek.Id, key.Id, err))
								continue
							}
						}

						newDekToBytesCache[dek.Id] = append([]byte(nil), dekBytes...)
						if dek.IsCurrent {
							newKeyToCurrentDEKCache[dek.KeyId] = dek.Id
						}
					}

					return pagination.Continue, nil
				},
			)
			if err != nil {
				merr = multierror.Append(merr, fmt.Errorf("failed to enumerate data encryption keys for key %q: %w", key.Id, err))
				continue
			}
		}

		return pagination.Continue, nil
	})

	if err != nil {
		merr = multierror.Append(merr, err)
	}

	// Identify all the keys used for the namespaces
	err = s.db.ListNamespacesBuilder().Enumerate(ctx, func(pr pagination.PageResult[database.Namespace]) (keepGoing pagination.KeepGoing, err error) {
		for _, ns := range pr.Results {
			if ns.KeyId != nil {
				newNamespaceToKeyCache[ns.Path] = *ns.KeyId
			}
		}

		return pagination.Continue, nil
	})

	if err != nil {
		merr = multierror.Append(merr, err)
	}

	s.mu.Lock()
	s.keyToKeyDataCache = newKeyToKeyDataCache
	s.dekToBytesCache = newDekToBytesCache
	s.keyToCurrentDEKCache = newKeyToCurrentDEKCache
	s.namespaceToKeyCache = newNamespaceToKeyCache
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

func (s *service) markSyncReady() {
	s.syncReadyOnce.Do(func() {
		close(s.syncReady)
	})
}

func (s *service) hasGlobalDataEncryptionKey() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dekID, hasGlobal := s.keyToCurrentDEKCache[globalEncryptionKeyID]
	if !hasGlobal {
		return false
	}

	_, hasGlobalBytes := s.dekToBytesCache[dekID]
	return hasGlobalBytes
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

	// Generate DEKs for tests before loading the runtime DEK cache.
	if s.cfg != nil && s.cfg.GetRoot() != nil {
		root := s.cfg.GetRoot()
		originalPolicy := root.SystemAuth.DataEncryptionKeys
		testPolicy := &sconfig.DataEncryptionKeys{}
		if originalPolicy != nil {
			copied := *originalPolicy
			testPolicy = &copied
		}
		testPolicy.RotationInterval = &sconfig.HumanDuration{}
		root.SystemAuth.DataEncryptionKeys = testPolicy
		defer func() {
			root.SystemAuth.DataEncryptionKeys = originalPolicy
		}()
	}
	if err := generateDataEncryptionKeysToDatabase(ctx, s.cfg, s.db, s.logger, nil); err != nil {
		s.logger.Warn("test encrypt service: failed to generate data encryption keys", "error", err)
	}

	// Sync keys from database to memory
	if err := s.syncKeysFromDbToMemory(ctx); err != nil {
		s.logger.Warn("test encrypt service: failed to sync keys from db to memory", "error", err)
	}

	s.markSyncReady()
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
		err := s.syncKeysFromDbToMemory(ctx)
		if err != nil {
			s.logger.Warn("encrypt service: initial key sync failed", "error", err)
		}

		if err == nil && s.hasGlobalDataEncryptionKey() {
			s.markSyncReady()
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

// getKeyIdForNamespace returns the key_ for the given namespace. If that namespace is not configured it falls
// back to parent namespaces until reaching the global key.
func (s *service) getKeyIdForNamespace(namespacePath string) (apid.ID, error) {
	paths := namespace.SplitNamespacePathToPrefixes(namespacePath)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := len(paths) - 1; i >= 0; i-- {
		if id, ok := s.namespaceToKeyCache[paths[i]]; ok {
			return id, nil
		}
	}

	return globalEncryptionKeyID, nil
}

// getCurrentDataEncryptionKeyId returns the current DEK for the given key.
func (s *service) getCurrentDataEncryptionKeyId(keyId apid.ID) (apid.ID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dekId, ok := s.keyToCurrentDEKCache[keyId]
	if !ok {
		return "", fmt.Errorf("no current data encryption key for key %s", keyId)
	}

	return dekId, nil
}

// getDataEncryptionKeyBytes returns the plaintext DEK bytes for the given dek_id.
func (s *service) getDataEncryptionKeyBytes(dekID apid.ID) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.dekToBytesCache[dekID]

	if ok {
		return append([]byte(nil), data...), nil
	}

	return nil, fmt.Errorf("data encryption key %s not found in cache", dekID)
}

// getAllKeyBytes returns all cached DEK bytes for trying decryption.
func (s *service) getAllKeyBytes() [][]byte {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([][]byte, 0, len(s.dekToBytesCache))
	for _, data := range s.dekToBytesCache {
		keys = append(keys, append([]byte(nil), data...))
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

// encryptForKey encrypts data with the current DEK of the specified key. The caller is assumed to
// have validated that the cache sync has completed.
func (s *service) encryptForKey(keyId apid.ID, data []byte) (encfield.EncryptedField, error) {
	dekId, err := s.getCurrentDataEncryptionKeyId(keyId)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	keyBytes, err := s.getDataEncryptionKeyBytes(dekId)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	encryptedData, err := encryptWithKey(keyBytes, data)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return encfield.EncryptedField{ID: dekId, Data: encodedData}, nil
}

// EncryptGlobal encrypts raw bytes with the current global key.
func (s *service) EncryptGlobal(ctx context.Context, data []byte) (encfield.EncryptedField, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return encfield.EncryptedField{}, err
	}
	return s.encryptForKey(globalEncryptionKeyID, data)
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

func (s *service) EncryptKeyForNamespace(ctx context.Context, namespacePath string, keyData []byte) (encfield.EncryptedField, error) {
	if namespacePath == namespace.RootNamespace {
		return s.EncryptGlobal(ctx, keyData)
	}

	// Keys are always encrypted with the parent namespace key to avoid creating a dependency cycle.
	parentNamespace := namespace.NamespaceParentPath(namespacePath)
	return s.EncryptForNamespace(ctx, parentNamespace, keyData)
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

// Decrypt decrypts an EncryptedField using the DEK ID embedded in the field. It assumes that the cache
// sync has completed.
func (s *service) decrypt(ef encfield.EncryptedField) ([]byte, error) {
	if ef.IsZero() {
		return nil, nil
	}

	keyBytes, err := s.getDataEncryptionKeyBytes(ef.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get data encryption key %s: %w", ef.ID, err)
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
// target DEK. If the field is already encrypted with the target DEK, it is
// returned unchanged.
func (s *service) ReEncryptField(ctx context.Context, ef encfield.EncryptedField, targetDEKId apid.ID) (encfield.EncryptedField, error) {
	if err := s.ensureSynced(ctx); err != nil {
		return encfield.EncryptedField{}, err
	}

	if ef.ID == targetDEKId {
		return ef, nil
	}

	plaintext, err := s.decrypt(ef)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	keyBytes, err := s.getDataEncryptionKeyBytes(targetDEKId)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	encrypted, err := encryptWithKey(keyBytes, plaintext)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	return encfield.EncryptedField{ID: targetDEKId, Data: base64.StdEncoding.EncodeToString(encrypted)}, nil
}

// SyncKeysFromDbToMemory forces a refresh of the in-memory key caches from the database.
func (s *service) SyncKeysFromDbToMemory(ctx context.Context) error {
	if err := s.syncKeysFromDbToMemory(ctx); err != nil {
		return err
	}

	if s.hasGlobalDataEncryptionKey() {
		s.markSyncReady()
	}

	return nil
}

// DecryptString decrypts an EncryptedField using the key ID embedded in the field.
func (s *service) DecryptString(ctx context.Context, ef encfield.EncryptedField) (string, error) {
	decryptedData, err := s.Decrypt(ctx, ef)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}
