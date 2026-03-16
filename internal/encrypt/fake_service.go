package encrypt

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
)

type fakeService struct {
	doBase64Encode bool
}

var fakeEncryptionKeyVersionId = apid.ID("ekv_fake")

// NewFakeEncryptService returns an encrypt service that does not encrypt or decrypt anything.
func NewFakeEncryptService(doBase64Encode bool) E {
	return &fakeService{
		doBase64Encode: doBase64Encode,
	}
}

// EncryptForKey encrypts data with the current version of the specified key.
func (s *fakeService) EncryptForKey(ctx context.Context, ekId apid.ID, data []byte) (encfield.EncryptedField, error) {
	return encfield.EncryptedField{
		ID:   fakeEncryptionKeyVersionId,
		Data: string(data),
	}, nil
}

// EncryptStringForKey encrypts a string with the current version of the specified key.
func (s *fakeService) EncryptStringForKey(ctx context.Context, ekId apid.ID, data string) (encfield.EncryptedField, error) {
	return s.EncryptForKey(ctx, ekId, []byte(data))
}

// EncryptGlobal encrypts raw bytes with the current global key.
func (s *fakeService) EncryptGlobal(ctx context.Context, data []byte) (encfield.EncryptedField, error) {
	return s.EncryptForKey(ctx, globalEncryptionKeyID, data)
}

// EncryptStringGlobal encrypts a string with the current global key.
func (s *fakeService) EncryptStringGlobal(ctx context.Context, data string) (encfield.EncryptedField, error) {
	return s.EncryptGlobal(ctx, []byte(data))
}

func (s *fakeService) EncryptForNamespace(ctx context.Context, _ string, data []byte) (encfield.EncryptedField, error) {
	return s.EncryptGlobal(ctx, data)
}

func (s *fakeService) EncryptStringForNamespace(ctx context.Context, namespacePath string, data string) (encfield.EncryptedField, error) {
	return s.EncryptForNamespace(ctx, namespacePath, []byte(data))
}

func (s *fakeService) EncryptForEntity(ctx context.Context, entity NamespacedEntity, data []byte) (encfield.EncryptedField, error) {
	return s.EncryptForNamespace(ctx, entity.GetNamespace(), data)
}

func (s *fakeService) EncryptStringForEntity(ctx context.Context, entity NamespacedEntity, data string) (encfield.EncryptedField, error) {
	return s.EncryptForEntity(ctx, entity, []byte(data))
}

// Decrypt decrypts an EncryptedField using the key ID embedded in the field.
func (s *fakeService) Decrypt(ctx context.Context, ef encfield.EncryptedField) ([]byte, error) {
	if ef.ID != fakeEncryptionKeyVersionId {
		return nil, fmt.Errorf("fake encryption service can only decrypt data encrypted with fake key version")
	}
	return []byte(ef.Data), nil
}

// DecryptString decrypts an EncryptedField using the key ID embedded in the field.
func (s *fakeService) DecryptString(ctx context.Context, ef encfield.EncryptedField) (string, error) {
	decryptedData, err := s.Decrypt(ctx, ef)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}

func (s *fakeService) ReEncryptField(ctx context.Context, ef encfield.EncryptedField, targetEkvId apid.ID) (encfield.EncryptedField, error) {
	if ef.ID == targetEkvId {
		return ef, nil
	}

	plaintext, err := s.Decrypt(ctx, ef)
	if err != nil {
		return encfield.EncryptedField{}, err
	}

	return encfield.EncryptedField{ID: targetEkvId, Data: string(plaintext)}, nil
}

func (s *fakeService) SyncKeysFromDbToMemory(ctx context.Context) error {
	return nil
}

func (s *fakeService) Start() {}

func (s *fakeService) Shutdown() {}
