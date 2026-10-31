package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	authcore "github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	dbmock "github.com/rmorlok/authproxy/internal/database/mock"
	"github.com/rmorlok/authproxy/internal/encfield"
	encryptmock "github.com/rmorlok/authproxy/internal/encrypt/mock"
	authschema "github.com/rmorlok/authproxy/internal/schema/auth"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// toolAuthorizationFixture supplies one captured revision with strict mocks;
// any management reread, extra decryption, or HTTP client setup is unexpected.
type toolAuthorizationFixture struct {
	service    *service
	db         *dbmock.MockDB
	encrypt    *encryptmock.MockE
	snapshot   *database.ToolExecutionSnapshot
	definition toolschema.ToolDefinition
	plaintext  []byte
}

// newToolAuthorizationFixture creates distinct ciphertext for each source so
// authorization must select the Tool definition rather than other private data.
func newToolAuthorizationFixture(t *testing.T, verbs []string) *toolAuthorizationFixture {
	t.Helper()
	s, db, _, _, _, encryption := FullMockService(t, gomock.NewController(t))
	connectionID := apid.MustParse("cxn_01example0000001")
	connectorID := apid.MustParse("cxr_01example0000001")
	snapshot := &database.ToolExecutionSnapshot{
		Revision: database.ToolRevision{
			ToolId: apid.MustParse("tol_01example0000001"), Revision: 7, ConnectionId: connectionID,
			EncryptedDefinition: encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "tool-definition"},
			SourceGeneration:    util.ToPtr(uint64(3)),
		},
		Connection: database.Connection{
			Id: connectionID, Namespace: "root.product.owner", State: database.ConnectionStateDisabled,
			ConnectorId: connectorID, ConnectorGeneration: 4, HealthState: database.ConnectionHealthStateHealthy,
			EncryptedConfiguration: &encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "connection-configuration"},
		},
		Connector: database.ConnectorWithDefinition{
			Id: connectorID, Namespace: "root.product", Generation: 4,
			EncryptedDefinition: encfield.EncryptedField{ID: apid.New(apid.PrefixDataEncryptionKey), Data: "connector-definition"},
		},
		ToolSetId: apid.New(apid.PrefixToolSet),
	}
	definition := toolschema.ToolDefinition{
		Description: "Captured executable definition", Verbs: verbs,
		InputSchema: common.RawJSON(`{"type":"object"}`),
		ProxyHTTP:   &toolschema.ProxyHTTP{Method: "GET", URL: "https://example.test/contact"},
	}
	plaintext, err := json.Marshal(definition)
	require.NoError(t, err)
	return &toolAuthorizationFixture{s, db, encryption, snapshot, definition, plaintext}
}

// expectCapturedDefinition permits exactly one snapshot read and one decryption.
func (f *toolAuthorizationFixture) expectCapturedDefinition() {
	gomock.InOrder(
		f.db.EXPECT().GetToolExecutionSnapshot(gomock.Any(), f.snapshot.Revision.ToolId).Return(f.snapshot, nil),
		f.encrypt.EXPECT().Decrypt(gomock.Any(), f.snapshot.Revision.EncryptedDefinition).Return(f.plaintext, nil),
	)
}

// toolInvocationGrant grants exact verbs on connections below the product namespace.
func toolInvocationGrant(verbs ...string) []authschema.Permission {
	return []authschema.Permission{{Namespace: "root.product.**", Resources: []string{"connections"}, Verbs: verbs}}
}

// toolInvocationAuth models the persisted actor grant and independent JWT restriction.
func toolInvocationAuth(actor, token []authschema.Permission) *authcore.RequestAuth {
	return authcore.NewAuthenticatedRequestAuthWithPermissions(&authcore.Actor{
		Id: apid.New(apid.PrefixActor), Namespace: "root", Permissions: actor,
	}, token)
}

// TestAuthorizeToolInvocationAliasIntersection requires one same captured alias
// to pass both actor and JWT permissions; proxy permission grants no Tool alias.
func TestAuthorizeToolInvocationAliasIntersection(t *testing.T) {
	const read, lookup = "tool:contacts.read", "tool:contacts.lookup"
	for _, test := range []struct {
		name         string
		actor, token []authschema.Permission
		aliases      []string
		allowed      bool
	}{
		{"tool grant without proxy", toolInvocationGrant(read), nil, nil, true},
		{"same alias", toolInvocationGrant(read), toolInvocationGrant(read), nil, true},
		{"different aliases", toolInvocationGrant(read), toolInvocationGrant(lookup), nil, false},
		{"second alias sufficient", toolInvocationGrant(read, lookup), toolInvocationGrant(lookup), nil, true},
		{"broader JWT cannot raise actor", toolInvocationGrant(read), toolInvocationGrant(read, lookup), nil, true},
		{"proxy alone", toolInvocationGrant("proxy"), nil, nil, false},
		{"proxy-only JWT", toolInvocationGrant(read, "proxy"), toolInvocationGrant("proxy"), nil, false},
		{"verb wildcard", toolInvocationGrant("*"), toolInvocationGrant(lookup), nil, true},
		{"broad admin narrowed JWT", authschema.AllPermissions(), toolInvocationGrant(read), nil, true},
		{"broad admin excluded by JWT", authschema.AllPermissions(), toolInvocationGrant("tool:other"), nil, false},
		{"prefix is not wildcard", toolInvocationGrant("tool:*"), nil, nil, false},
		{"literal star alias", toolInvocationGrant("tool:*"), nil, []string{"tool:*"}, true},
		{"empty captured aliases fail closed", authschema.AllPermissions(), nil, []string{}, false},
		{"no actor grants", nil, toolInvocationGrant(read), nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			aliases := test.aliases
			if aliases == nil {
				aliases = []string{read, lookup}
			}
			f := newToolAuthorizationFixture(t, aliases)
			f.expectCapturedDefinition()
			auth := toolInvocationAuth(test.actor, test.token)

			// Only captured ciphertext is read; disabled-state readiness belongs later.
			got, err := f.service.authorizeToolInvocation(context.Background(), auth, f.snapshot.Revision.ToolId, nil)
			if !test.allowed {
				require.ErrorIs(t, err, ErrNotFound)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Same(t, f.snapshot, got.snapshot)
			require.Equal(t, f.definition, got.definition)
			if test.name == "tool grant without proxy" {
				require.False(t, auth.Allows(f.snapshot.Connection.Namespace, "connections", "proxy", f.snapshot.Connection.Id.String()))
			}
		})
	}
}

// TestAuthorizeToolInvocationTargetScope applies namespace, resource, and ID
// restrictions to the captured connection rather than the Tool or its ToolSet.
func TestAuthorizeToolInvocationTargetScope(t *testing.T) {
	for _, test := range []struct {
		name    string
		change  func(*authcore.RequestAuth, *database.ToolExecutionSnapshot)
		allowed bool
	}{
		{"exact connection", func(a *authcore.RequestAuth, s *database.ToolExecutionSnapshot) {
			a.GetActor().Permissions[0].ResourceIds = []string{s.Connection.Id.String()}
		}, true},
		{"Tool ID is not connection ID", func(a *authcore.RequestAuth, s *database.ToolExecutionSnapshot) {
			a.GetActor().Permissions[0].ResourceIds = []string{s.Revision.ToolId.String()}
		}, false},
		{"JWT connection restriction", func(a *authcore.RequestAuth, _ *database.ToolExecutionSnapshot) {
			p := toolInvocationGrant("tool:read")
			p[0].ResourceIds = []string{apid.New(apid.PrefixConnection).String()}
			a.SetPermissions(p)
		}, false},
		{"exact namespace", func(a *authcore.RequestAuth, s *database.ToolExecutionSnapshot) {
			a.GetActor().Permissions[0].Namespace = s.Connection.Namespace
		}, true},
		{"parent without descendants", func(a *authcore.RequestAuth, _ *database.ToolExecutionSnapshot) {
			a.GetActor().Permissions[0].Namespace = "root.product"
		}, false},
		{"JWT other namespace", func(a *authcore.RequestAuth, _ *database.ToolExecutionSnapshot) {
			p := toolInvocationGrant("tool:read")
			p[0].Namespace = "root.other.**"
			a.SetPermissions(p)
		}, false},
		{"actor namespace bounds broad grants", func(a *authcore.RequestAuth, _ *database.ToolExecutionSnapshot) {
			a.GetActor().Namespace = "root.other"
		}, false},
		{"Tool resource does not grant connection verb", func(a *authcore.RequestAuth, _ *database.ToolExecutionSnapshot) {
			a.GetActor().Permissions[0].Resources = []string{"tools"}
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Change only one target restriction while retaining the same declared alias.
			f := newToolAuthorizationFixture(t, []string{"tool:read"})
			f.expectCapturedDefinition()
			auth := toolInvocationAuth(toolInvocationGrant("tool:read"), nil)
			test.change(auth, f.snapshot)
			// Authorization must evaluate that restriction against the captured connection.
			got, err := f.service.authorizeToolInvocation(context.Background(), auth, f.snapshot.Revision.ToolId, nil)
			if test.allowed {
				require.NoError(t, err)
				require.NotNil(t, got)
			} else {
				require.ErrorIs(t, err, ErrNotFound)
				require.Nil(t, got)
			}
		})
	}
}

// TestAuthorizeToolInvocationRevisionCheck follows alias authorization so an
// unauthorized caller cannot use expectedRevision to inspect a hidden Tool.
func TestAuthorizeToolInvocationRevisionCheck(t *testing.T) {
	for _, test := range []struct {
		name     string
		verb     string
		expected uint64
		want     error
	}{
		{"matching captured revision", "tool:read", 7, nil},
		{"authorized stale revision", "tool:read", 6, ErrToolRevisionChanged},
		{"denied matching revision", "tool:other", 7, ErrNotFound},
		{"denied stale revision", "tool:other", 6, ErrNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The stored revision stays fixed while caller permissions and expectation vary.
			f := newToolAuthorizationFixture(t, []string{"tool:read"})
			f.expectCapturedDefinition()
			got, err := f.service.authorizeToolInvocation(context.Background(), toolInvocationAuth(toolInvocationGrant(test.verb), nil), f.snapshot.Revision.ToolId, &test.expected)
			if test.want == nil {
				require.NoError(t, err)
				require.Equal(t, uint64(7), got.snapshot.Revision.Revision)
			} else {
				require.ErrorIs(t, err, test.want)
				require.Nil(t, got)
			}
		})
	}
}

// TestAuthorizeToolInvocationPreconditions rejects invalid or unauthenticated
// requests and preexisting cancellation without any database or encryption call.
func TestAuthorizeToolInvocationPreconditions(t *testing.T) {
	for _, name := range []string{"nil auth", "unauthenticated", "missing ID", "wrong ID", "zero revision", "canceled"} {
		t.Run(name, func(t *testing.T) {
			f := newToolAuthorizationFixture(t, []string{"tool:read"})
			ctx := context.Background()
			auth := toolInvocationAuth(toolInvocationGrant("tool:read"), nil)
			id, want := f.snapshot.Revision.ToolId, ErrNotFound
			var expected *uint64
			switch name {
			case "nil auth":
				auth = nil
			case "unauthenticated":
				auth = authcore.NewUnauthenticatedRequestAuth()
			case "missing ID":
				id, want = apid.Nil, ErrInvalidArgument
			case "wrong ID":
				id, want = f.snapshot.Connection.Id, ErrInvalidArgument
			case "zero revision":
				expected, want = util.ToPtr(uint64(0)), ErrInvalidArgument
			case "canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx, want = canceled, context.Canceled
			}
			// No mock expectations permit I/O for these rejected inputs.
			got, err := f.service.authorizeToolInvocation(ctx, auth, id, expected)
			require.ErrorIs(t, err, want)
			require.Nil(t, got)
		})
	}
}

// TestAuthorizeToolInvocationSourceFailures preserves storage/decryption errors,
// rejects invalid persisted JSON, and stops promptly between cancellable steps.
func TestAuthorizeToolInvocationSourceFailures(t *testing.T) {
	privateFailure := errors.New("private storage or decryption failure")
	for _, name := range []string{"missing snapshot", "database failure", "decrypt failure", "malformed definition", "unknown field", "canceled after read", "canceled after decrypt"} {
		t.Run(name, func(t *testing.T) {
			f := newToolAuthorizationFixture(t, []string{"tool:read"})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var want error
			snapshot, readErr := f.snapshot, error(nil)
			switch name {
			case "missing snapshot":
				snapshot, readErr, want = nil, database.ErrNotFound, ErrNotFound
			case "database failure":
				snapshot, readErr, want = nil, privateFailure, privateFailure
			}
			f.db.EXPECT().GetToolExecutionSnapshot(gomock.Any(), f.snapshot.Revision.ToolId).DoAndReturn(func(context.Context, apid.ID) (*database.ToolExecutionSnapshot, error) {
				if name == "canceled after read" {
					cancel()
				}
				return snapshot, readErr
			})
			if readErr == nil && name != "canceled after read" {
				plaintext, decryptErr := f.plaintext, error(nil)
				switch name {
				case "decrypt failure":
					plaintext, decryptErr, want = nil, privateFailure, privateFailure
				case "malformed definition":
					plaintext = []byte(`{"verbs":`)
				case "unknown field":
					plaintext = []byte(strings.Replace(string(plaintext), "{", `{"unknown":true,`, 1))
				}
				f.encrypt.EXPECT().Decrypt(gomock.Any(), f.snapshot.Revision.EncryptedDefinition).DoAndReturn(func(context.Context, encfield.EncryptedField) ([]byte, error) {
					if name == "canceled after decrypt" {
						cancel()
					}
					return plaintext, decryptErr
				})
			}
			if strings.HasPrefix(name, "canceled") {
				want = context.Canceled
			}

			// No failure returns a partially authorized revision or starts HTTP work.
			got, err := f.service.authorizeToolInvocation(ctx, toolInvocationAuth(toolInvocationGrant("tool:read"), nil), f.snapshot.Revision.ToolId, nil)
			require.Error(t, err)
			if want != nil {
				require.ErrorIs(t, err, want)
			}
			require.Nil(t, got)
		})
	}
}
