package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
	"github.com/rmorlok/authproxy/internal/util"
)

// ConnectorVersion is a wrapper for the lower level database equivalent that handles things like decrypting the
// configuration, checking upgradability, etc.
type ConnectorVersion struct {
	database.ConnectorVersion

	s     *service
	defMu sync.RWMutex
	def   *cschema.Connector
	l     *slog.Logger
}

func wrapConnectorVersion(cv database.ConnectorVersion, s *service) *ConnectorVersion {
	return &ConnectorVersion{
		ConnectorVersion: cv,
		s:                s,
		l: aplog.NewBuilder(s.logger).
			WithNamespace(cv.Namespace).
			WithConnectorId(cv.Id).
			WithConnectorVersion(cv.Version).
			Build(),
	}
}

func (cv *ConnectorVersion) GetId() uuid.UUID {
	return cv.ConnectorVersion.Id
}

func (cv *ConnectorVersion) GetNamespace() string {
	return cv.ConnectorVersion.Namespace
}

func (cv *ConnectorVersion) GetVersion() uint64 {
	return cv.ConnectorVersion.Version
}

func (cv *ConnectorVersion) GetState() database.ConnectorVersionState {
	return cv.ConnectorVersion.State
}

func (cv *ConnectorVersion) GetType() string {
	return cv.ConnectorVersion.Type
}

func (cv *ConnectorVersion) GetHash() string {
	return cv.ConnectorVersion.Hash
}

func (cv *ConnectorVersion) GetDefinition() *cschema.Connector {
	return util.Must(cv.getDefinition())
}

func (cv *ConnectorVersion) GetCreatedAt() time.Time {
	return cv.ConnectorVersion.CreatedAt
}

func (cv *ConnectorVersion) GetUpdatedAt() time.Time {
	return cv.ConnectorVersion.UpdatedAt
}

func (cv *ConnectorVersion) GetLabels() map[string]string {
	return cv.ConnectorVersion.Labels
}

func (cv *ConnectorVersion) getDefinition() (*cschema.Connector, error) {
	cv.defMu.RLock()
	defer cv.defMu.RUnlock()
	if cv.def == nil {
		decrypted, err := cv.s.encrypt.DecryptStringForConnector(context.Background(), cv, cv.EncryptedDefinition)
		if err != nil {
			return nil, err
		}

		var def cschema.Connector
		err = json.Unmarshal([]byte(decrypted), &def)
		if err != nil {
			return nil, err
		}
		cv.def = &def
	}

	return cv.def, nil
}

func (cv *ConnectorVersion) setDefinition(def *cschema.Connector) error {
	cv.defMu.Lock()
	defer cv.defMu.Unlock()

	jsonBytes, err := json.Marshal(def)
	if err != nil {
		return err
	}

	encrypted, err := cv.s.encrypt.EncryptStringForConnector(context.Background(), cv, string(jsonBytes))
	if err != nil {
		return err
	}
	cv.Hash = def.Hash()
	cv.EncryptedDefinition = encrypted
	cv.def = def

	return nil
}

func (cv *ConnectorVersion) Logger() *slog.Logger {
	return cv.l
}

var _ iface.ConnectorVersion = (*ConnectorVersion)(nil)
var _ aplog.HasLogger = (*ConnectorVersion)(nil)
