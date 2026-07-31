package core

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util"
)

// Connector hydrates a logical connector and one selected definition version
// with decrypted configuration and compiled JavaScript behavior.
type Connector struct {
	database.ConnectorWithDefinition
	Hash string

	s     *service
	defMu sync.RWMutex
	def   *cschema.Connector

	jsMu     sync.RWMutex
	jsLib    *apjs.Library
	jsLibErr error
	jsLoaded bool

	l *slog.Logger
}

func wrapConnector(cv database.ConnectorWithDefinition, s *service) *Connector {
	return &Connector{
		ConnectorWithDefinition: cv,
		s:                       s,
		l: aplog.NewBuilder(s.logger).
			WithNamespace(cv.Namespace).
			WithConnectorId(cv.Id).
			WithConnectorVersion(cv.Version).
			Build(),
	}
}

func (cv *Connector) GetId() apid.ID {
	return cv.ConnectorWithDefinition.Id
}

func (cv *Connector) GetNamespace() string {
	return cv.ConnectorWithDefinition.Namespace
}

func (cv *Connector) GetVersion() uint64 {
	return cv.ConnectorWithDefinition.Version
}

func (cv *Connector) GetState() database.ConnectorDefinitionVersionState {
	return cv.ConnectorWithDefinition.State
}

func (cv *Connector) GetHash() string {
	return util.Must(cv.getHash())
}

func (cv *Connector) GetDefinition() *cschema.Connector {
	return util.Must(cv.getDefinition())
}

func (cv *Connector) GetCreatedAt() time.Time {
	return cv.ConnectorWithDefinition.CreatedAt
}

func (cv *Connector) GetUpdatedAt() time.Time {
	return cv.ConnectorWithDefinition.UpdatedAt
}

func (cv *Connector) GetLabels() map[string]string {
	return cv.ConnectorWithDefinition.Labels
}

func (cv *Connector) GetAnnotations() map[string]string {
	return cv.ConnectorWithDefinition.Annotations
}

func (cv *Connector) getDefinition() (*cschema.Connector, error) {
	cv.defMu.RLock()
	if cv.def != nil {
		defer cv.defMu.RUnlock()
		return cv.def, nil
	}
	cv.defMu.RUnlock()

	cv.defMu.Lock()
	defer cv.defMu.Unlock()
	if cv.def == nil {
		decrypted, err := cv.s.encrypt.DecryptString(context.Background(), cv.ConnectorWithDefinition.EncryptedDefinition)
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

func (cv *Connector) getHash() (string, error) {
	if cv.Hash != "" {
		return cv.Hash, nil
	}
	decrypted, err := cv.s.encrypt.DecryptString(context.Background(), cv.ConnectorWithDefinition.EncryptedDefinition)
	if err != nil {
		return "", err
	}
	hash := sha1.Sum([]byte(decrypted))
	return hex.EncodeToString(hash[:])[:7], nil
}

func (cv *Connector) setDefinition(def *cschema.Connector) error {
	cv.defMu.Lock()

	jsonBytes, err := json.Marshal(def)
	if err != nil {
		cv.defMu.Unlock()
		return err
	}

	encrypted, err := cv.s.encrypt.EncryptStringForEntity(context.Background(), cv, string(jsonBytes))
	if err != nil {
		cv.defMu.Unlock()
		return err
	}
	cv.Hash = def.Hash()
	cv.ConnectorWithDefinition.EncryptedDefinition = encrypted
	cv.def = def
	cv.defMu.Unlock()

	cv.resetJavascriptLibrary()

	return nil
}

func (c *Connector) getJavascriptLibrary() (*apjs.Library, error) {
	c.jsMu.RLock()
	if c.jsLoaded {
		defer c.jsMu.RUnlock()
		return c.jsLib, c.jsLibErr
	}
	c.jsMu.RUnlock()

	def, err := c.getDefinition()
	if err != nil {
		return nil, err
	}
	jsLib, jsLibErr := apjs.CompileLibrary(def.Javascript)

	c.jsMu.Lock()
	defer c.jsMu.Unlock()
	if !c.jsLoaded {
		c.jsLib = jsLib
		c.jsLibErr = jsLibErr
		c.jsLoaded = true
	}

	return c.jsLib, c.jsLibErr
}

func (c *Connector) resetJavascriptLibrary() {
	c.jsMu.Lock()
	defer c.jsMu.Unlock()
	c.jsLib = nil
	c.jsLibErr = nil
	c.jsLoaded = false
}

func (cv *Connector) Logger() *slog.Logger {
	return cv.l
}

var _ iface.Connector = (*Connector)(nil)
var _ aplog.HasLogger = (*Connector)(nil)
