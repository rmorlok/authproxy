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
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolschema "github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/require"
)

// toolPublicationFixture supplies a standalone script and strict dependencies;
// configuration, credentials, HTTP setup, and persistence are never expected.
func toolPublicationFixture(t *testing.T, source string) *toolAuthorizationFixture {
	t.Helper()
	f := newToolAuthorizationFixture(t, []string{"tool:read"})
	f.definition.ProxyHTTP = nil
	f.definition.Javascript = &source
	return f
}

// expectToolPublicationLibrary allows only the exact pinned generation and its
// encrypted definition to be loaded, never a primary-generation lookup.
func expectToolPublicationLibrary(t *testing.T, f *toolAuthorizationFixture, source string) {
	t.Helper()
	plaintext, err := json.Marshal(map[string]any{"javascript": source})
	require.NoError(t, err)
	gomock.InOrder(
		f.db.EXPECT().GetConnectorGeneration(gomock.Any(), f.snapshot.Connection.ConnectorId, f.snapshot.Connection.ConnectorGeneration).
			Return(&f.snapshot.Connector, nil),
		f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
			Return(string(plaintext), nil),
	)
}

// TestToolJavaScriptPublicationPinnedLibrary validates pure initialization with
// the selected helpers even when the connection is not ready for invocation.
func TestToolJavaScriptPublicationPinnedLibrary(t *testing.T) {
	for _, state := range []database.ConnectionState{
		database.ConnectionStateSetup, database.ConnectionStateDisabled, database.ConnectionStateConfigured,
	} {
		t.Run(string(state), func(t *testing.T) {
			// No auth definition or decrypted connection data is needed to validate code.
			f := toolPublicationFixture(t, `
				if (selected() !== "pinned") throw "wrong library";
				if (typeof params !== "undefined" || typeof context !== "undefined" ||
				    typeof cfg !== "undefined" || typeof labels !== "undefined" ||
				    typeof annotations !== "undefined" || typeof fetch !== "undefined" ||
				    typeof console !== "undefined") throw "unexpected capability";
				const result = toolResult({structuredContent: btoa("ready")});
				const failure = new ToolError("DECLINED", "Declined");
				async function execute() { throw "execute must not run at publication"; }
			`)
			f.snapshot.Connection.State = state
			expectToolPublicationLibrary(t, f, `function selected() { return "pinned"; }`)

			// The strict mocks reject extra reads or effects; only initialization runs.
			_, err := f.service.validateToolJavaScriptPublication(context.Background(), &f.definition, &f.snapshot.Connection)
			require.NoError(t, err)
		})
	}
}

// TestToolJavaScriptPublicationInvalidCode rejects syntax, host collisions, and
// initialization failures without exposing authored source or thrown values.
func TestToolJavaScriptPublicationInvalidCode(t *testing.T) {
	for name, code := range map[string]struct{ script, library string }{
		"script syntax":      {`async function privatePublicationSource( {`, ""},
		"library syntax":     {`async function execute() {}`, `function privatePublicationSource( {`},
		"synchronous entry":  {`function execute() {}`, ""},
		"missing entry":      {`const privatePublicationSource = 1;`, ""},
		"host collision":     {`const fetch = 1; async function execute() {}`, ""},
		"library collision":  {`async function execute() {}`, `const ToolError = 1;`},
		"script throw":       {`throw "privatePublicationSource"; async function execute() {}`, ""},
		"library throw":      {`async function execute() {}`, `throw "privatePublicationSource";`},
		"initialization I/O": {`context.connections[0].fetch("https://privatePublicationSource.invalid"); async function execute() {}`, ""},
		"direct I/O":         {`fetch("https://privatePublicationSource.invalid"); async function execute() {}`, ""},
		"microtask I/O":      {`Promise.resolve().then(() => fetch("https://privatePublicationSource.invalid")); async function execute() {}`, ""},
		"rejected promise":   {`Promise.reject("privatePublicationSource"); async function execute() {}`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			// Only helper loading is permitted; invalid initialization cannot send requests.
			f := toolPublicationFixture(t, code.script)
			expectToolPublicationLibrary(t, f, code.library)
			_, err := f.service.validateToolJavaScriptPublication(context.Background(), &f.definition, &f.snapshot.Connection)
			require.ErrorIs(t, err, ErrInvalidArgument)
			require.NotContains(t, err.Error(), "privatePublicationSource")
		})
	}
}

// TestToolJavaScriptPublicationSourceLimit counts the raw helper text, including
// whitespace which a compiled empty library might otherwise omit.
func TestToolJavaScriptPublicationSourceLimit(t *testing.T) {
	const source = `async function execute() { return true; }`
	for _, extra := range []int{0, 1} {
		name := "at combined limit"
		if extra != 0 {
			name = "over combined limit"
		}
		t.Run(name, func(t *testing.T) {
			f := toolPublicationFixture(t, source)
			expectToolPublicationLibrary(t, f, strings.Repeat(" ", (256<<10)-len(source)+extra))

			// A raw-byte overflow fails before helper parsing or VM initialization.
			_, err := f.service.validateToolJavaScriptPublication(context.Background(), &f.definition, &f.snapshot.Connection)
			if extra == 0 {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrInvalidArgument)
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			}
		})
	}
}

// TestToolJavaScriptPublicationSchema rejects an invalid executable schema
// before loading helpers or starting JavaScript initialization.
func TestToolJavaScriptPublicationSchema(t *testing.T) {
	f := toolPublicationFixture(t, `async function execute() { return true; }`)
	f.definition.OutputSchema = common.RawJSON(`{"$ref":"#/missing"}`)
	// The unresolved schema cannot become part of a published executable revision.
	_, err := f.service.validateToolJavaScriptPublication(context.Background(), &f.definition, &f.snapshot.Connection)
	require.ErrorIs(t, err, ErrInvalidArgument)
}

// TestToolJavaScriptPublicationStackCap applies the Tool's lower stack allowance
// during initialization as well as eventual invocation.
func TestToolJavaScriptPublicationStackCap(t *testing.T) {
	for _, capped := range []bool{false, true} {
		name := "publication ceiling"
		if capped {
			name = "authored lower ceiling"
		}
		t.Run(name, func(t *testing.T) {
			f := toolPublicationFixture(t, `
				function descend(n) { if (n > 0) descend(n - 1); }
				descend(64);
				async function execute() { return true; }
			`)
			if capped {
				f.definition.Limits = &toolschema.ExecutionLimits{MaxStackDepth: util.ToPtr(16)}
			}
			expectToolPublicationLibrary(t, f, "")

			// The same code fits the fixed profile but exceeds its authored lower cap.
			_, err := f.service.validateToolJavaScriptPublication(context.Background(), &f.definition, &f.snapshot.Connection)
			if capped {
				require.ErrorIs(t, err, ErrInvalidArgument)
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestToolJavaScriptPublicationPreparationAllowance keeps dependency loading
// independent of the Tool's authored initialization timeout.
func TestToolJavaScriptPublicationPreparationAllowance(t *testing.T) {
	f := toolPublicationFixture(t, `async function execute() { return true; }`)
	const authoredTimeout = 100 * time.Millisecond
	f.definition.Limits = &toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(authoredTimeout / time.Millisecond))}
	before := time.Now()
	var deadline time.Time
	f.db.EXPECT().GetConnectorGeneration(gomock.Any(), f.snapshot.Connection.ConnectorId, uint64(4)).
		DoAndReturn(func(workCtx context.Context, _ apid.ID, _ uint64) (*database.ConnectorWithDefinition, error) {
			var ok bool
			deadline, ok = workCtx.Deadline()
			require.True(t, ok)
			// The preparation deadline uses the fixed allowance, not the authored cap.
			require.WithinRange(t, deadline, before.Add(5*time.Second), time.Now().Add(5*time.Second))
			return &f.snapshot.Connector, nil
		})
	f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
		DoAndReturn(func(workCtx context.Context, _ encfield.EncryptedField) (string, error) {
			got, ok := workCtx.Deadline()
			require.True(t, ok)
			require.Equal(t, deadline, got)
			// Finish loading after an authored timeout would already have expired.
			timer := time.NewTimer(authoredTimeout + 10*time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				require.NoError(t, workCtx.Err())
				return `{}`, nil
			case <-workCtx.Done():
				return "", workCtx.Err()
			}
		})

	// Trivial initialization still receives its full independent authored allowance.
	_, err := f.service.validateToolJavaScriptPublication(context.Background(), &f.definition, &f.snapshot.Connection)
	require.NoError(t, err)
}

// TestToolJavaScriptPublicationInterruption distinguishes caller cancellation
// and dependency deadlines from exhaustion of the initialization allowance.
func TestToolJavaScriptPublicationInterruption(t *testing.T) {
	for _, name := range []string{"already canceled", "caller cancels load", "caller deadline during load", "busy initialization"} {
		t.Run(name, func(t *testing.T) {
			f := toolPublicationFixture(t, `async function execute() { return true; }`)
			f.definition.Limits = &toolschema.ExecutionLimits{TimeoutMillis: util.ToPtr(int64(250))}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "caller deadline during load" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 50*time.Millisecond)
				defer stop()
			}
			switch name {
			case "already canceled":
				cancel()
			case "busy initialization":
				*f.definition.Javascript = `while (true) {} async function execute() {}`
				expectToolPublicationLibrary(t, f, "")
			default:
				// Cancellation reaches dependency loading without running authored code.
				f.db.EXPECT().GetConnectorGeneration(gomock.Any(), f.snapshot.Connection.ConnectorId, uint64(4)).
					Return(&f.snapshot.Connector, nil)
				f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).
					DoAndReturn(func(workCtx context.Context, _ encfield.EncryptedField) (string, error) {
						if name == "caller cancels load" {
							cancel()
						}
						<-workCtx.Done()
						return "", workCtx.Err()
					})
			}

			// Only actual initialization exhaustion is reported as invalid authored code.
			_, err := f.service.validateToolJavaScriptPublication(ctx, &f.definition, &f.snapshot.Connection)
			if name == "busy initialization" {
				require.NoError(t, ctx.Err())
				require.ErrorIs(t, err, ErrInvalidArgument)
				require.ErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			} else {
				require.ErrorIs(t, err, ctx.Err())
				require.NotErrorIs(t, err, ErrInvalidArgument)
				require.NotErrorIs(t, err, apjsruntime.ErrLimitExceeded)
			}
		})
	}
}

// TestToolJavaScriptPublicationDependencyFailures preserves infrastructure
// failures instead of reporting valid authored definitions as invalid input.
func TestToolJavaScriptPublicationDependencyFailures(t *testing.T) {
	for _, name := range []string{"connector read", "definition decrypt", "connector timeout", "decryption timeout"} {
		t.Run(name, func(t *testing.T) {
			f := toolPublicationFixture(t, `async function execute() {}`)
			failure := errors.New("dependency unavailable")
			if strings.HasSuffix(name, "timeout") {
				failure = context.DeadlineExceeded
			}
			read := f.db.EXPECT().GetConnectorGeneration(gomock.Any(), f.snapshot.Connection.ConnectorId, uint64(4))
			if name == "connector read" || name == "connector timeout" {
				read.Return(nil, failure)
			} else {
				read.Return(&f.snapshot.Connector, nil)
				f.encrypt.EXPECT().DecryptString(gomock.Any(), f.snapshot.Connector.EncryptedDefinition).Return("", failure)
			}

			// Callers can retain their existing retry/status handling for dependency errors.
			_, err := f.service.validateToolJavaScriptPublication(context.Background(), &f.definition, &f.snapshot.Connection)
			require.ErrorIs(t, err, failure)
			require.NotErrorIs(t, err, ErrInvalidArgument)
			require.NotErrorIs(t, err, apjsruntime.ErrLimitExceeded)
		})
	}
}
