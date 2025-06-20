package connectors

import (
	"context"
	"encoding/json"
	cfg "github.com/rmorlok/authproxy/config/connectors"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/util"
	"sync"
)

// ConnectorVersion is a wrapper for the lower level database equivalent that handles things like decrypting the
// configuration, checking upgradability, etc.
type ConnectorVersion struct {
	database.ConnectorVersion

	mu  sync.RWMutex
	s   *service
	def *cfg.Connector
}

func wrapConnectorVersion(cv database.ConnectorVersion, s *service) *ConnectorVersion {
	return &ConnectorVersion{
		ConnectorVersion: cv,
		s:                s,
	}
}

func (cv *ConnectorVersion) GetDefinition() *cfg.Connector {
	return util.Must(cv.getDefinition())
}

func (cv *ConnectorVersion) getDefinition() (*cfg.Connector, error) {
	cv.mu.RLock()
	defer cv.mu.RUnlock()
	if cv.def == nil {
		decrypted, err := cv.s.encrypt.DecryptStringForConnector(context.Background(), cv.ConnectorVersion, cv.EncryptedDefinition)
		if err != nil {
			return nil, err
		}

		var def cfg.Connector
		err = json.Unmarshal([]byte(decrypted), &def)
		if err != nil {
			return nil, err
		}
		cv.def = &def
	}

	return cv.def, nil
}

func (cv *ConnectorVersion) setDefinition(def *cfg.Connector) error {
	cv.mu.Lock()
	defer cv.mu.Unlock()

	jsonBytes, err := json.Marshal(def)
	if err != nil {
		return err
	}

	encrypted, err := cv.s.encrypt.EncryptStringForConnector(context.Background(), cv.ConnectorVersion, string(jsonBytes))
	if err != nil {
		return err
	}
	cv.Hash = def.Hash()
	cv.EncryptedDefinition = encrypted
	cv.def = def

	return nil
}
