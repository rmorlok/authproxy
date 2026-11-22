package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apjs"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/encrypt"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type namespaceHolder struct {
	namespace string
}

func (n *namespaceHolder) GetNamespace() string {
	return n.namespace
}

func TestWrapConnector(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := apid.New(apid.PrefixActor)
	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Generation:          1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorGenerationStateDraft,
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"},
	}

	// Test
	c := wrapConnector(dbConnector, s)

	// Verify
	assert.Equal(t, dbConnector, c.ConnectorWithDefinition)
	assert.Equal(t, s, c.s)
	assert.Nil(t, c.def)
}

func TestConnector_GetDefinition(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := apid.New(apid.PrefixActor)
	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Generation:          1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorGenerationStateDraft,
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"},
	}

	c := wrapConnector(dbConnector, s)

	// Create a connector definition
	def := &cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	defJSON, _ := json.Marshal(def)

	// Set up expectations for the encrypt service
	mockEncrypt.EXPECT().
		DecryptString(gomock.Any(), encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}).
		Return(string(defJSON), nil)

	// Test
	result := c.GetDefinition()

	// Verify
	assert.Equal(t, def.DisplayName, result.DisplayName)
	assert.Equal(t, def.Description, result.Description)

	// Test caching - should not call decrypt again
	result2 := c.GetDefinition()
	assert.Equal(t, result, result2)
}

func TestConnector_GetResource(t *testing.T) {
	createdAt := time.Date(2026, time.September, 5, 14, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	id := apid.MustParse("cxr_testresource00001")
	definition := &cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
		Description: "A test connector",
	}
	labels := database.Labels{"team": "platform"}
	annotations := database.Annotations{"example.com/owner": "integrations"}
	c := &Connector{
		ConnectorWithDefinition: database.ConnectorWithDefinition{
			Id:          id,
			Name:        "test-connector",
			Namespace:   "root.acme",
			Generation:  3,
			State:       database.ConnectorGenerationStateArchived,
			Labels:      labels,
			Annotations: annotations,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		},
		def: definition,
	}

	resource := c.GetResource()
	require.Equal(t, cschema.ConnectorKind, resource.Kind)
	require.Equal(t, id.String(), resource.Metadata.ID)
	require.Equal(t, "test-connector", string(resource.Metadata.Name))
	require.Equal(t, "root.acme", resource.Metadata.Namespace)
	require.Equal(t, uint64(3), resource.Metadata.Generation)
	require.Equal(t, createdAt, *resource.Metadata.CreatedAt)
	require.Equal(t, updatedAt, *resource.Metadata.UpdatedAt)
	require.Equal(t, cschema.ConnectorReleaseStatePrimary, resource.Spec.Release.DesiredState)
	require.Equal(t, cschema.ConnectorReleaseStateArchived, resource.Status.Release.State)
	require.Equal(t, definition, &resource.Spec.Definition)
	require.NoError(t, resource.ValidateFor(meta.ValidationModeResponse, nil))

	resource.Metadata.Labels["team"] = "changed"
	resource.Metadata.Annotations["example.com/owner"] = "changed"
	resource.Spec.Definition.Description = "changed"
	require.Equal(t, "platform", labels["team"])
	require.Equal(t, "integrations", annotations["example.com/owner"])
	require.Equal(t, "A test connector", definition.Description)
}

func TestConnector_GetHashDerivesFromEncryptedDefinition(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	encryptedDefinition := encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"}
	def := &cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
	}
	defJSON := util.Must(json.Marshal(def))
	mockEncrypt.EXPECT().
		DecryptString(gomock.Any(), encryptedDefinition).
		Return(string(defJSON), nil)

	c := wrapConnector(database.ConnectorWithDefinition{
		Id:                  apid.New(apid.PrefixConnector),
		Generation:          1,
		EncryptedDefinition: encryptedDefinition,
	}, &service{encrypt: mockEncrypt, logger: aplog.NewNoopLogger()})

	require.Equal(t, def.Hash(), c.GetHash())
}

func TestConnector_SetDefinition(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockEncrypt := encryptmock.NewMockE(ctrl)
	s := &service{
		encrypt: mockEncrypt,
		logger:  aplog.NewNoopLogger(),
	}

	connectorId := apid.New(apid.PrefixActor)
	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Generation:          1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorGenerationStateDraft,
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "encrypted-data"},
	}

	c := wrapConnector(dbConnector, s)

	// Create a connector definition
	def := &cschema.ConnectorDefinition{
		DisplayName: "Test Connector",
		Description: "A test connector",
	}

	// Calculate expected hash
	expectedHash := def.Hash()

	// Set up expectations for the encrypt service
	newEncryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "new-encrypted-data"}
	mockEncrypt.EXPECT().
		EncryptStringForEntity(
			gomock.Any(),
			gomock.Any(),
			`{"auth":null,"description":"A test connector","displayName":"Test Connector","logo":null}`).
		Return(newEncryptedDef, nil)

	// Test
	err := c.setDefinition(def)

	// Verify
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, c.Hash)
	assert.Equal(t, newEncryptedDef, c.EncryptedDefinition)
	assert.Equal(t, def, c.def)
}

func TestConnector_SetDefinitionResetsJavascriptLibrary(t *testing.T) {
	c := NewTestConnector(cschema.ConnectorDefinition{
		Javascript: `function isUpdated() { return false; }`,
	})

	library, err := c.getJavascriptLibrary()
	require.NoError(t, err)
	ok, err := library.NewContext(nil).EvaluateBoolean(`isUpdated()`)
	require.NoError(t, err)
	assert.False(t, ok)

	err = c.setDefinition(&cschema.ConnectorDefinition{
		Javascript: `function isUpdated() { return true; }`,
	})
	require.NoError(t, err)

	library, err = c.getJavascriptLibrary()
	require.NoError(t, err)
	ok, err = library.NewContext(nil).EvaluateBoolean(`isUpdated()`)
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestConnectorBoundedJavascriptLibrarySourceBudget checks the raw combined
// allowance before either compiling a library or returning a cached program.
func TestConnectorBoundedJavascriptLibrarySourceBudget(t *testing.T) {
	for _, test := range []struct {
		name       string
		library    string
		expression string
	}{
		{"combined source", `function allowed() { return true; }`, `allowed()`},
		{"expression only", "", `true`},
		{"library only", `const eligible = true;`, ""},
		{"raw whitespace", strings.Repeat(" ", 32), `true`},
		{"UTF-8 bytes", `const name = "é";`, `true`},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A one-byte overflow must be rejected before compilation populates the cache.
			connector := NewTestConnector(cschema.ConnectorDefinition{Javascript: test.library})
			limits := apjs.ConnectorPredicateLimits()
			limits.MaxSourceBytes = len(test.library) + len(test.expression) - 1
			library, err := connector.getBoundedJavascriptLibrary(context.Background(), test.expression, limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, library)
			require.False(t, connector.jsLoaded)

			// The exact combined size fits, and a repeated call reuses the compiled library.
			limits.MaxSourceBytes++
			library, err = connector.getBoundedJavascriptLibrary(context.Background(), test.expression, limits)
			require.NoError(t, err)
			require.True(t, connector.jsLoaded)
			cached, err := connector.getBoundedJavascriptLibrary(context.Background(), test.expression, limits)
			require.NoError(t, err)
			require.Same(t, library, cached)

			// A later caller's tighter policy still rejects the raw source on a cache hit.
			limits.MaxSourceBytes--
			cached, err = connector.getBoundedJavascriptLibrary(context.Background(), test.expression, limits)
			require.ErrorIs(t, err, jsruntime.ErrLimitExceeded)
			require.Nil(t, cached)
			require.Same(t, library, connector.jsLib)
		})
	}
}

// TestConnectorBoundedJavascriptLibraryLoadRetry preserves infrastructure causes
// and permits a fresh caller to retry when definition admission did not succeed.
func TestConnectorBoundedJavascriptLibraryLoadRetry(t *testing.T) {
	for _, name := range []string{"decrypt failure", "decode failure", "canceled during decrypt", "canceled decrypt failure"} {
		t.Run(name, func(t *testing.T) {
			// Fail the first contextual load without populating either cache.
			connector := NewTestConnector(cschema.ConnectorDefinition{})
			encryption := encryptmock.NewMockE(gomock.NewController(t))
			connector.s.encrypt = encryption
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			loadFailure := errors.New("decryption failed")
			encryption.EXPECT().DecryptString(ctx, connector.EncryptedDefinition).DoAndReturn(func(context.Context, encfield.EncryptedField) (string, error) {
				switch name {
				case "decrypt failure":
					return "", loadFailure
				case "decode failure":
					return "{", nil
				case "canceled during decrypt":
					cancel()
					return `{}`, nil
				default:
					cancel()
					return "", loadFailure
				}
			})
			limits := apjs.ConnectorPredicateLimits()
			library, err := connector.getBoundedJavascriptLibrary(ctx, "true", limits)
			require.Error(t, err)
			require.Nil(t, library)
			require.Nil(t, connector.def)
			require.False(t, connector.jsLoaded)
			switch name {
			case "decrypt failure":
				require.ErrorIs(t, err, loadFailure)
			case "decode failure":
				var syntaxError *json.SyntaxError
				require.ErrorAs(t, err, &syntaxError)
			default:
				require.ErrorIs(t, err, context.Canceled)
			}

			// A live retry decrypts once; later library loads use both successful caches.
			encryption.EXPECT().DecryptString(context.Background(), connector.EncryptedDefinition).
				Return(`{"javascript":"const eligible = true;"}`, nil)
			library, err = connector.getBoundedJavascriptLibrary(context.Background(), "true", limits)
			require.NoError(t, err)
			cached, err := connector.getBoundedJavascriptLibrary(context.Background(), "true", limits)
			require.NoError(t, err)
			require.Same(t, library, cached)
		})
	}
}

// TestConnectorBoundedJavascriptLibraryErrors keeps authored parser diagnostics
// private while honoring cancellation and input policy before loading source.
func TestConnectorBoundedJavascriptLibraryErrors(t *testing.T) {
	connector := NewTestConnector(cschema.ConnectorDefinition{Javascript: `const privateLibraryDetail = ;`})
	limits := apjs.ConnectorPredicateLimits()

	// Invalid admission cannot decrypt the definition or populate the program cache.
	library, err := connector.getBoundedJavascriptLibrary(nil, "true", limits)
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, library)
	library, err = connector.getBoundedJavascriptLibrary(context.Background(), "true", jsruntime.Limits{})
	require.ErrorIs(t, err, jsruntime.ErrInvalidInput)
	require.Nil(t, library)
	require.Nil(t, connector.def)
	require.False(t, connector.jsLoaded)

	// Both fresh and cached parser failures expose the same safe classification.
	for range 2 {
		library, err = connector.getBoundedJavascriptLibrary(context.Background(), "true", limits)
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
		require.Equal(t, jsruntime.ErrInvalidSource.Error(), err.Error())
		require.Nil(t, library)
		require.True(t, connector.jsLoaded)
	}

	// Cancellation takes precedence over invalid limits and the cached parse error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, policy := range []jsruntime.Limits{limits, {}} {
		library, err = connector.getBoundedJavascriptLibrary(ctx, "true", policy)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, library)
	}
}

// NewTestConnector creates a hydrated test connector using the provided definition.
func NewTestConnector(c cschema.ConnectorDefinition) *Connector {
	e := encrypt.NewFakeEncryptService(false)
	connectorId := apid.New(apid.PrefixActor)
	encryptedDefinition, err := e.EncryptStringForEntity(context.Background(), &namespaceHolder{namespace: "root"}, string(util.Must(json.Marshal(c))))
	if err != nil {
		panic(err)
	}

	dbConnector := database.ConnectorWithDefinition{
		Id:                  connectorId,
		Generation:          1,
		Labels:              map[string]string{"type": "test-connector"},
		State:               database.ConnectorGenerationStatePrimary,
		EncryptedDefinition: encryptedDefinition,
	}

	return wrapConnector(dbConnector, &service{encrypt: e, logger: aplog.NewNoopLogger()})
}

// TestConnectorDefinitionContextRetry keeps failed or canceled loads out of the
// cache, while successful admission preloading serves the legacy lazy getters.
func TestConnectorDefinitionContextRetry(t *testing.T) {
	for _, name := range []string{"decrypt failure", "decode failure", "canceled during decrypt"} {
		t.Run(name, func(t *testing.T) {
			// The first contextual load fails without poisoning the captured generation.
			s, _, _, _, _, encryption := FullMockService(t, gomock.NewController(t))
			stored := database.ConnectorWithDefinition{
				Id: apid.New(apid.PrefixConnector), Generation: 3,
				EncryptedDefinition: encfield.EncryptedField{Data: "captured-definition"},
			}
			connector := wrapConnector(stored, s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			loadFailure := errors.New("decryption failed")
			encryption.EXPECT().DecryptString(ctx, stored.EncryptedDefinition).DoAndReturn(func(context.Context, encfield.EncryptedField) (string, error) {
				switch name {
				case "decrypt failure":
					return "", loadFailure
				case "decode failure":
					return "{", nil
				default:
					cancel()
					return `{"displayName":"Canceled"}`, nil
				}
			})
			definition, err := connector.getDefinitionWithContext(ctx)
			require.Error(t, err)
			require.Nil(t, definition)
			require.Nil(t, connector.def)
			if name == "decrypt failure" {
				require.ErrorIs(t, err, loadFailure)
			} else if name == "canceled during decrypt" {
				require.ErrorIs(t, err, context.Canceled)
			}

			// A new live context can retry, then future getters use the cached result.
			encryption.EXPECT().DecryptString(context.Background(), stored.EncryptedDefinition).
				Return(`{"displayName":"Captured"}`, nil)
			definition, err = connector.getDefinitionWithContext(context.Background())
			require.NoError(t, err)
			require.Equal(t, "Captured", definition.DisplayName)
			require.Same(t, definition, connector.GetDefinition())

			// Even a cached definition does not make a canceled contextual call succeed.
			cancel()
			definition, err = connector.getDefinitionWithContext(ctx)
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, definition)
		})
	}
}
