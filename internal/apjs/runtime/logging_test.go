package runtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/dop251/goja"
	jsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/stretchr/testify/require"
)

// loggingInput supplies an explicit logger without granting HTTP or copying any
// invocation data into the authored logging surface.
func loggingInput(logger *slog.Logger) func(context.Context, *goja.Runtime, *jsruntime.AsyncInvocation) error {
	return func(_ context.Context, _ *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		invocation.Params, invocation.Context = goja.Undefined(), goja.Undefined()
		invocation.Logger = logger
		return nil
	}
}

// loggingRecords decodes complete records emitted by the real JSON handler,
// preserving nested fields so assertions cover the actual structured contract.
func loggingRecords(t *testing.T, buffer *bytes.Buffer) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(buffer)
	var records []map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		require.NoError(t, err)
		records = append(records, record)
	}
}

// TestLoggingLevelsAndStructuredFields emits detached data beneath the fixed
// authored namespace while retaining attribution supplied only by the host.
func TestLoggingLevelsAndStructuredFields(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug})).With(
		"namespace", "root.owner", "connection_id", "cxn_trusted", "trace_id", "trusted-trace",
	)
	program := compileAsync(t, `async function execute() {
		const fields = {
			namespace: "authored-owner", connection_id: "authored-connection", trace_id: "authored-trace",
			msg: "authored-message", level: "ERROR", time: "authored-time",
			nested: {items: [null, true, 3.25, "café", "\udfff"]}, "\ud800": "replacement"
		};
		const returned = console.debug("debug", fields);
		fields.nested.items[0] = "later mutation";
		console.info("info");
		console.log("log", undefined);
		console.warn("warning\ud800");
		console.error("error", Object.create(null));
		return [typeof returned, typeof fetch];
	}`, nil)

	// The explicit capability accepts five levels without adding network access.
	result, err := program.Evaluate(context.Background(), loggingInput(logger))
	require.NoError(t, err)
	require.JSONEq(t, `["undefined","undefined"]`, string(result))
	records := loggingRecords(t, &buffer)
	require.Len(t, records, 5)
	for index, level := range []string{"DEBUG", "INFO", "INFO", "WARN", "ERROR"} {
		require.Equal(t, level, records[index]["level"])
		require.Equal(t, "JavaScript authored log", records[index]["msg"])
		require.Equal(t, "root.owner", records[index]["namespace"])
		require.Equal(t, "cxn_trusted", records[index]["connection_id"])
		require.Equal(t, "trusted-trace", records[index]["trace_id"])
	}

	// Authored attribution-like keys remain nested and strings use scalar Unicode.
	first := records[0]["javascript"].(map[string]any)
	require.Equal(t, "debug", first["message"])
	fields, err := json.Marshal(first["fields"])
	require.NoError(t, err)
	require.JSONEq(t, `{"namespace":"authored-owner","connection_id":"authored-connection",
		"trace_id":"authored-trace","msg":"authored-message","level":"ERROR","time":"authored-time",
		"nested":{"items":[null,true,3.25,"café","�"]},"�":"replacement"}`, string(fields))
	for _, index := range []int{1, 2, 3} {
		require.NotContains(t, records[index]["javascript"], "fields")
	}
	require.Equal(t, "warning�", records[3]["javascript"].(map[string]any)["message"])
	require.Equal(t, map[string]any{}, records[4]["javascript"].(map[string]any)["fields"])
}

// TestLoggingDoesNotCaptureInvocationData confines each record to the authored
// message/fields and host attribution instead of copying inputs or results.
func TestLoggingDoesNotCaptureInvocationData(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	program := compileAsync(t, `async function execute(params, context) {
		console.info("selected", {count: 1});
		return "private-result";
	}`, nil)

	// Private invocation data is available to the script but not implicitly logged.
	result, err := program.Evaluate(context.Background(), func(_ context.Context, vm *goja.Runtime, invocation *jsruntime.AsyncInvocation) error {
		invocation.Params, invocation.Context = vm.ToValue("private-arguments"), vm.ToValue("private-configuration")
		invocation.Logger = logger
		return nil
	})
	require.NoError(t, err)
	require.JSONEq(t, `"private-result"`, string(result))
	require.NotContains(t, buffer.String(), "private-")
	require.Len(t, loggingRecords(t, &buffer), 1)
}

// TestLoggingAvailability keeps logging opt-in for async execution and absent
// from validation and pure expressions, independently of pure utility access.
func TestLoggingAvailability(t *testing.T) {
	program := compileAsync(t, `const initial = typeof console;
		async function execute() {return [initial, typeof console, typeof URL];}`, nil)
	// Validation and ordinary async evaluation receive no logger implicitly.
	require.NoError(t, program.Validate(context.Background()))
	result, err := program.Evaluate(context.Background(), loggingInput(nil))
	require.NoError(t, err)
	require.JSONEq(t, `["undefined","undefined","function"]`, string(result))

	// Pure expressions retain all pure helpers without an authored log capability.
	expression := compileExpression(t, `[typeof console, typeof URL]`)
	require.JSONEq(t, `["undefined","function"]`, string(evaluateExpression(t, expression, nil)))
	for _, source := range []string{
		`const console = {}; async function execute() {}`,
		`var console; async function execute() {}`,
		`async function execute() {} function console() {}`,
	} {
		_, err := jsruntime.CompileAsync(source, nil, nil, runtimeLimits())
		require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
	}
	_, err = jsruntime.CompileAsync(`async function execute() {}`, asyncLibrary(t, `var console;`), nil, runtimeLimits())
	require.ErrorIs(t, err, jsruntime.ErrInvalidSource)
}

// TestLoggingProtectedBoundMethods protects the global/native surface while
// allowing a borrowed method to retain its original host capability.
func TestLoggingProtectedBoundMethods(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	program := compileAsync(t, `async function execute() {
		let rejected = 0;
		for (const change of [
			() => {globalThis.console = {};},
			() => {console.info = () => {};},
			() => {console.extra = true;},
			() => Object.defineProperty(console, "info", {value: () => {}}),
			() => Object.setPrototypeOf(console, {})
		]) {try {change();} catch (error) {if (error instanceof TypeError) rejected++;}}
		const descriptor = Object.getOwnPropertyDescriptor(globalThis, "console");
		const borrowed = console.info;
		borrowed.call({namespace: "fake-owner"}, "borrowed", {ok: true});
		return [rejected, Object.getPrototypeOf(console) === null, Object.isFrozen(console),
			descriptor.writable, descriptor.configurable, descriptor.enumerable];
	}`, nil)

	// Neither authored mutation nor a different this receiver can replace the host.
	result, err := program.Evaluate(context.Background(), loggingInput(logger))
	require.NoError(t, err)
	require.JSONEq(t, `[5,true,true,false,false,false]`, string(result))
	records := loggingRecords(t, &buffer)
	require.Len(t, records, 1)
	require.Equal(t, "borrowed", records[0]["javascript"].(map[string]any)["message"])
}

// TestLoggingRejectsMalformedRecords rejects unsupported data without invoking
// authored getters, proxy traps, coercion, or toJSON, and allows later valid logs.
func TestLoggingRejectsMalformedRecords(t *testing.T) {
	for name, expression := range map[string]string{
		"missing message":       `console.info()`,
		"undefined message":     `console.info(undefined)`,
		"null message":          `console.info(null)`,
		"numeric message":       `console.info(1)`,
		"boxed message":         `console.info(new String("private-data"))`,
		"coercible message":     `console.info({toString() {touched++; return "private-data";}})`,
		"proxy message":         `console.info(new Proxy({}, {get() {touched++; throw "private-data";}}))`,
		"extra arguments":       `console.info("private-data", {}, 1)`,
		"null fields":           `console.info("private-data", null)`,
		"array fields":          `console.info("private-data", [])`,
		"primitive fields":      `console.info("private-data", "value")`,
		"custom prototype":      `console.info("private-data", Object.create({secret: true}))`,
		"getter":                `console.info("private-data", {get secret() {touched++; return 1;}})`,
		"nested getter":         `console.info("private-data", {nested: {get secret() {touched++; return 1;}}})`,
		"proxy":                 `console.info("private-data", new Proxy({}, {ownKeys() {touched++; throw "private-data";}}))`,
		"revoked proxy":         `console.info("private-data", revoked)`,
		"own toJSON":            `console.info("private-data", {toJSON() {touched++; return {};}})`,
		"nonfinite number":      `console.info("private-data", {value: NaN})`,
		"undefined field":       `console.info("private-data", {value: undefined})`,
		"symbol field":          `console.info("private-data", {value: Symbol("private-data")})`,
		"symbol key":            `console.info("private-data", {[Symbol("private-data")]: 1})`,
		"sparse nested array":   `console.info("private-data", {value: Array(2)})`,
		"cycle":                 `console.info("private-data", cycle)`,
		"unsupported URL value": `console.info("private-data", {url: new URL("https://private-data.test/")})`,
	} {
		t.Run(name, func(t *testing.T) {
			var buffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buffer, nil))
			program := compileAsync(t, `async function execute() {
				let touched = 0, caught = false, message = "";
				const cycle = {}; cycle.self = cycle;
				const pair = Proxy.revocable({}, {}); pair.revoke(); const revoked = pair.proxy;
				try {`+expression+`;} catch (error) {caught = error instanceof TypeError; message = error.message;}
				console.info("recovered");
				return [caught, touched, message];
			}`, nil)

			// Refused records never reach the sink and do not poison valid later calls.
			result, err := program.Evaluate(context.Background(), loggingInput(logger))
			require.NoError(t, err)
			var returned []any
			require.NoError(t, json.Unmarshal(result, &returned))
			require.Equal(t, true, returned[0])
			require.Equal(t, float64(0), returned[1])
			require.Equal(t, "Invalid JavaScript log record", returned[2])
			require.NotContains(t, buffer.String(), "private-data")
			records := loggingRecords(t, &buffer)
			require.Len(t, records, 1)
			require.Equal(t, "recovered", records[0]["javascript"].(map[string]any)["message"])
		})
	}
}

// TestLoggingCapturesIntrinsics uses the pristine encoder even after authored
// code replaces reflection/JSON helpers and adds inherited serialization hooks.
func TestLoggingCapturesIntrinsics(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, nil))
	program := compileAsync(t, `async function execute() {
		let touched = 0;
		const fields = JSON.parse('{"__proto__":{"polluted":true},"nested":{"value":3}}');
		Object.getOwnPropertyDescriptor = () => {touched++; throw "private-hook";};
		Reflect.ownKeys = () => {touched++; throw "private-hook";};
		JSON.stringify = () => {touched++; throw "private-hook";};
		Object.prototype.toJSON = () => {touched++; throw "private-hook";};
		console.info("captured", fields);
		return touched;
	}`, nil)

	// Both log conversion and the ordinary result remain independent of those hooks.
	result, err := program.Evaluate(context.Background(), loggingInput(logger))
	require.NoError(t, err)
	require.JSONEq(t, `0`, string(result))
	records := loggingRecords(t, &buffer)
	require.Len(t, records, 1)
	fields, err := json.Marshal(records[0]["javascript"].(map[string]any)["fields"])
	require.NoError(t, err)
	require.JSONEq(t, `{"__proto__":{"polluted":true},"nested":{"value":3}}`, string(fields))
}

// TestLoggingDisabledLevelsStillValidate keeps record validation independent of
// handler level filtering while sending only the enabled authored record.
func TestLoggingDisabledLevelsStillValidate(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelError}))
	program := compileAsync(t, `async function execute() {
		let touched = 0, rejected = false;
		console.debug("filtered", {valid: true});
		try {console.info("private-data", {get value() {touched++; return true;}});}
		catch (error) {rejected = error instanceof TypeError;}
		console.error("visible", {valid: true});
		return [rejected, touched];
	}`, nil)

	// The disabled malformed call is refused without hooks; the valid one is silent.
	result, err := program.Evaluate(context.Background(), loggingInput(logger))
	require.NoError(t, err)
	require.JSONEq(t, `[true,0]`, string(result))
	records := loggingRecords(t, &buffer)
	require.Len(t, records, 1)
	require.Equal(t, "ERROR", records[0]["level"])
	require.Equal(t, "visible", records[0]["javascript"].(map[string]any)["message"])
}
