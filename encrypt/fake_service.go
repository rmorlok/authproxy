package encrypt

import (
	"context"
	"encoding/base64"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/database"
)

type fakeService struct {
	doBase64Encode bool
}

// NewFakeEncryptService returns an encrypt service that does not encrypt or decrypt anything.
func NewFakeEncryptService(doBase64Encode bool) E {
	return &fakeService{
		doBase64Encode: doBase64Encode,
	}
}

func (s *fakeService) EncryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	return data, nil
}

func (s *fakeService) EncryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error) {
	return s.EncryptGlobal(ctx, data)
}

func (s *fakeService) EncryptForConnector(ctx context.Context, connection database.ConnectorVersion, data []byte) ([]byte, error) {
	return s.EncryptGlobal(ctx, data)
}

func (s *fakeService) DecryptGlobal(ctx context.Context, data []byte) ([]byte, error) {
	return data, nil
}

func (s *fakeService) DecryptForConnection(ctx context.Context, connection database.Connection, data []byte) ([]byte, error) {
	return s.DecryptGlobal(ctx, data)
}

func (s *fakeService) DecryptForConnector(ctx context.Context, cv database.ConnectorVersion, data []byte) ([]byte, error) {
	return s.DecryptGlobal(ctx, data)
}

func (s *fakeService) EncryptStringGlobal(ctx context.Context, data string) (string, error) {
	encryptedData, err := s.EncryptGlobal(ctx, []byte(data))
	if err != nil {
		return "", err
	}

	if !s.doBase64Encode {
		return string(encryptedData), nil
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return encodedData, nil
}

func (s *fakeService) EncryptStringForConnection(ctx context.Context, connection database.Connection, data string) (string, error) {
	encryptedData, err := s.EncryptForConnection(ctx, connection, []byte(data))
	if err != nil {
		return "", err
	}

	if !s.doBase64Encode {
		return string(encryptedData), nil
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return encodedData, nil
}

func (s *fakeService) EncryptStringForConnector(ctx context.Context, cv database.ConnectorVersion, data string) (string, error) {
	encryptedData, err := s.EncryptForConnector(ctx, cv, []byte(data))
	if err != nil {
		return "", err
	}

	if !s.doBase64Encode {
		return string(encryptedData), nil
	}

	encodedData := base64.StdEncoding.EncodeToString(encryptedData)
	return encodedData, nil
}

func (s *fakeService) DecryptStringGlobal(ctx context.Context, base64Data string) (string, error) {
	if !s.doBase64Encode {
		return base64Data, nil
	}

	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode base64 string")
	}

	decryptedData, err := s.DecryptGlobal(ctx, decodedData)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}

func (s *fakeService) DecryptStringForConnection(ctx context.Context, connection database.Connection, base64Data string) (string, error) {
	if !s.doBase64Encode {
		return base64Data, nil
	}

	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode base64 string")
	}

	decryptedData, err := s.DecryptForConnection(ctx, connection, decodedData)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}

func (s *fakeService) DecryptStringForConnector(ctx context.Context, cv database.ConnectorVersion, base64Data string) (string, error) {
	if !s.doBase64Encode {
		return base64Data, nil
	}

	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode base64 string")
	}

	decryptedData, err := s.DecryptForConnector(ctx, cv, decodedData)
	if err != nil {
		return "", err
	}

	return string(decryptedData), nil
}
