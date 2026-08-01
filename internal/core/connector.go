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
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
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

func wrapConnector(c database.ConnectorWithDefinition, s *service) *Connector {
	return &Connector{
		ConnectorWithDefinition: c,
		s:                       s,
		l: aplog.NewBuilder(s.logger).
			WithNamespace(c.Namespace).
			WithConnectorId(c.Id).
			WithConnectorVersion(c.Version).
			Build(),
	}
}

func (c *Connector) GetId() apid.ID {
	return c.ConnectorWithDefinition.Id
}

func (c *Connector) GetNamespace() string {
	return c.ConnectorWithDefinition.Namespace
}

func (c *Connector) GetName() scommon.ResourceName {
	return c.ConnectorWithDefinition.Name
}

func (c *Connector) GetVersion() uint64 {
	return c.ConnectorWithDefinition.Version
}

func (c *Connector) GetState() database.ConnectorDefinitionVersionState {
	return c.ConnectorWithDefinition.State
}

func (c *Connector) GetHash() string {
	return util.Must(c.getHash())
}

func (c *Connector) GetDefinition() *cschema.Connector {
	return util.Must(c.getDefinition())
}

func (c *Connector) GetCreatedAt() time.Time {
	return c.ConnectorWithDefinition.CreatedAt
}

func (c *Connector) GetUpdatedAt() time.Time {
	return c.ConnectorWithDefinition.UpdatedAt
}

func (c *Connector) GetLabels() map[string]string {
	return c.ConnectorWithDefinition.Labels
}

func (c *Connector) GetAnnotations() map[string]string {
	return c.ConnectorWithDefinition.Annotations
}

func (c *Connector) getDefinition() (*cschema.Connector, error) {
	c.defMu.RLock()
	if c.def != nil {
		defer c.defMu.RUnlock()
		return c.def, nil
	}
	c.defMu.RUnlock()

	c.defMu.Lock()
	defer c.defMu.Unlock()
	if c.def == nil {
		decrypted, err := c.s.encrypt.DecryptString(context.Background(), c.ConnectorWithDefinition.EncryptedDefinition)
		if err != nil {
			return nil, err
		}

		var def cschema.Connector
		err = json.Unmarshal([]byte(decrypted), &def)
		if err != nil {
			return nil, err
		}
		c.def = &def
	}

	return c.def, nil
}

func (c *Connector) getHash() (string, error) {
	if c.Hash != "" {
		return c.Hash, nil
	}
	decrypted, err := c.s.encrypt.DecryptString(context.Background(), c.ConnectorWithDefinition.EncryptedDefinition)
	if err != nil {
		return "", err
	}
	hash := sha1.Sum([]byte(decrypted))
	return hex.EncodeToString(hash[:])[:7], nil
}

func (c *Connector) setDefinition(def *cschema.Connector) error {
	c.defMu.Lock()

	jsonBytes, err := json.Marshal(def)
	if err != nil {
		c.defMu.Unlock()
		return err
	}

	encrypted, err := c.s.encrypt.EncryptStringForEntity(context.Background(), c, string(jsonBytes))
	if err != nil {
		c.defMu.Unlock()
		return err
	}
	c.Hash = def.Hash()
	c.ConnectorWithDefinition.EncryptedDefinition = encrypted
	c.def = def
	c.defMu.Unlock()

	c.resetJavascriptLibrary()

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

func (c *Connector) Logger() *slog.Logger {
	return c.l
}

var _ iface.Connector = (*Connector)(nil)
var _ aplog.HasLogger = (*Connector)(nil)
