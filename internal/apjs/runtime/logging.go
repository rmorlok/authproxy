package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/dop251/goja"
)

// logHost owns one invocation's optional authored logging capability. Only the
// VM owner accesses it; the supplied logger receives detached Go data and the
// execution context, never VM values or authored callbacks.
type logHost struct {
	host    *utilityHost   // Shared cancellation, work, and allocation accounting.
	encoder *outputEncoder // Pristine conversion intrinsics and per-record boundaries.
	freeze  goja.Callable  // Captured before preparation can modify the global Object.
	logger  *slog.Logger   // Trusted sink with caller-captured attribution; nil grants nothing.
	active  bool           // Enables emission only after initialization succeeds.
	closed  bool           // Permanently prevents emission during cleanup/result conversion.
}

// newLogHost captures intrinsics before adapter preparation or authored code.
// It does not install a console or enable a logging sink.
func newLogHost(ctx context.Context, vm *goja.Runtime, limits Limits) *logHost {
	freeze, _ := goja.AssertFunction(vm.Get("Object").ToObject(vm).Get("freeze"))
	return &logHost{
		host:    &utilityHost{ctx: ctx, vm: vm},
		encoder: newOutputEncoder(vm, limits), freeze: freeze,
	}
}

// install binds the restricted console surface to a supplied logger. Authored
// fields cannot replace host attributes: they live under the javascript group.
// No logger is selected implicitly, including when the default logger exists.
func (l *logHost) install(logger *slog.Logger) error {
	if logger == nil {
		return nil
	}
	if err := ChargeWork(l.host.ctx, 6); err != nil {
		return err
	}
	if err := ReserveAllocation(l.host.ctx, 6); err != nil {
		return err
	}
	console := l.host.vm.NewObject()
	if err := console.SetPrototype(nil); err != nil {
		return ErrInvalidInput
	}
	for name, level := range map[string]slog.Level{
		"debug": slog.LevelDebug, "info": slog.LevelInfo, "log": slog.LevelInfo,
		"warn": slog.LevelWarn, "error": slog.LevelError,
	} {
		if err := console.DefineDataProperty(name, l.host.vm.ToValue(l.method(level)),
			goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
			return ErrInvalidInput
		}
	}
	if _, err := l.freeze(goja.Undefined(), console); err != nil {
		return ErrInvalidInput
	}
	if err := l.host.vm.GlobalObject().DefineDataProperty("console", console,
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
		return ErrInvalidInput
	}
	l.logger = logger
	return nil
}

// start opens logging only after initialization and its Promise checks succeed.
// A closed host cannot be reopened, even by a later lifecycle callback.
func (l *logHost) start() {
	if !l.closed && l.logger != nil {
		l.active = true
	}
}

// close gates emission before cleanup and result interpretation. Previously
// emitted records remain observable even if the invocation later fails.
func (l *logHost) close() {
	l.active, l.closed = false, true
}

// method binds a fixed severity and sink independently of its JavaScript
// receiver. Every call validates and charges its payload, even when the sink's
// level filter discards the record. There is no formatting or object coercion.
func (l *logHost) method(level slog.Level) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		l.host.beginCall()
		if !l.active || l.closed {
			panic(l.host.vm.NewTypeError("JavaScript logging unavailable"))
		}
		if len(call.Arguments) < 1 || len(call.Arguments) > 2 {
			l.invalidInput()
		}
		if _, ok := call.Argument(0).(goja.String); !ok {
			l.invalidInput()
		}
		fields := call.Argument(1)
		if !goja.IsUndefined(fields) {
			object, ok := fields.(*goja.Object)
			if !ok || object.ExportType() != l.encoder.objectExportType || object.ClassName() != "Object" {
				l.invalidInput()
			}
		}

		// Each record gets fresh structural/serialized boundaries while all
		// intermediate storage and work consume the execution's shared ledger.
		l.host.reserveOutput(4) // Traversal and fixed record/group attribute slots.
		state := &outputState{encoder: l.encoder, ctx: l.host.ctx, active: make(map[*goja.Object]bool)}
		message := l.decode(state, call.Argument(0)).(string)
		attrs := []slog.Attr{slog.String("message", message)}
		if !goja.IsUndefined(fields) {
			attrs = append(attrs, slog.Any("fields", l.decode(state, fields)))
		}
		l.host.checkContext()

		// Handlers are trusted synchronous code and must return promptly. The
		// supplied context preserves the application's logging/trace pipeline.
		l.logger.LogAttrs(l.host.ctx, level, "JavaScript authored log",
			slog.Attr{Key: "javascript", Value: slog.GroupValue(attrs...)})
		l.host.checkContext()
		return goja.Undefined()
	}
}

// decode first uses strict bounded VM conversion, then decodes only those
// admitted bytes into ordinary Go JSON data. This keeps structured fields intact
// for both slog and telemetry handlers without exporting any VM object. Native
// JSON decoding has no cancellation hook; its bounded pass is checked on both
// sides. Lone surrogate escapes become U+FFFD, as in ordinary JSON decoding.
func (l *logHost) decode(state *outputState, value goja.Value) any {
	before := state.nodes
	encoded, err := state.encode(value)
	state.bytes = nil // The next field gets a new buffer but shares byte/node caps.
	if err != nil {
		l.host.checkContext()
		if errors.Is(err, ErrLimitExceeded) {
			l.host.failLimit()
		}
		l.invalidInput()
	}
	// Reserve decoded strings/container slots and the native decoding pass
	// before creating the detached graph. Units bound storage by fixed factors.
	l.host.consumeInput(len(encoded))
	l.host.reserveOutput(len(encoded))
	l.host.consumeInput(state.nodes - before)
	l.host.reserveOutput(state.nodes - before)
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		l.host.checkContext()
		l.invalidInput()
	}
	l.host.checkContext()
	return decoded
}

// invalidInput rejects malformed records with fixed text, without logging the
// attempted data, parser diagnostics, or errors raised by authored code.
func (l *logHost) invalidInput() {
	panic(l.host.vm.NewTypeError("Invalid JavaScript log record"))
}
