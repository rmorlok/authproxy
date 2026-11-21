package runtime

import (
	"context"

	"github.com/dop251/goja"
)

// UtilityPolicy selects pure native helpers and bounds their combined work in
// one VM. The zero value installs nothing. Enabled helpers require positive
// caps, separate from initial JSON input and final JSON output allowances.
// Programs copy this value with Limits; each evaluation gets fresh counters.
type UtilityPolicy struct {
	// Base64 enables atob and btoa during both initialization and execution.
	// Other planned utility globals are not installed by this flag.
	Base64 bool

	// MaxCalls counts all attempted helper calls, including malformed input.
	MaxCalls int

	// MaxInputBytes counts input code units before scanning or allocating. For
	// accepted base64/binary strings each code unit represents one ASCII/byte
	// value. Whitespace and invalid strings still consume their admitted length.
	MaxInputBytes int

	// MaxOutputBytes bounds aggregate reserved result bytes: ASCII characters
	// for btoa and binary-string code units for atob. Encoding buffers and Goja
	// UTF-16 storage use bounded multiples of these caps, not a heap quota.
	MaxOutputBytes int
}

// validate rejects unlimited work for enabled helpers. Caps are unused when
// no helpers are selected, so existing zero-valued execution policies still work.
func (p UtilityPolicy) validate() error {
	if p.Base64 && (p.MaxCalls <= 0 || p.MaxInputBytes <= 0 || p.MaxOutputBytes <= 0) {
		return ErrInvalidInput
	}
	return nil
}

// bindings lists enabled utility globals for async declaration checks. Their
// installation also protects the properties in pure expression/Run profiles.
func (p UtilityPolicy) bindings() []HostBinding {
	if p.Base64 {
		return []HostBinding{{Name: "atob"}, {Name: "btoa"}}
	}
	return nil
}

// utilityHost owns one VM's pure-helper accounting and cancellation context.
// Native callbacks run only on the VM owner. Counters never reset or refund
// admitted work; a limit failure interrupts JavaScript and remains sticky for Run.
type utilityHost struct {
	ctx         context.Context // Same deadline used by initialization and execution.
	vm          *goja.Runtime   // Creates native values and fixed, catchable errors.
	policy      UtilityPolicy   // Immutable caller-supplied aggregate allowances.
	calls       int             // Calls admitted before argument validation.
	inputBytes  int             // Input code units admitted before scanning/copying.
	outputBytes int             // Result bytes reserved before output allocation.
	err         error           // Sticky policy failure checked by Run on every exit.
}

// installUtilities binds selected native functions before any trusted adapter
// callback or authored initialization. It exposes no Go objects, I/O, or logger.
// Helpers are non-enumerable, non-writable, and non-configurable in every profile.
func installUtilities(ctx context.Context, vm *goja.Runtime, policy UtilityPolicy) (*utilityHost, error) {
	if !policy.Base64 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host := &utilityHost{ctx: ctx, vm: vm, policy: policy}
	for name, helper := range map[string]func(goja.FunctionCall) goja.Value{
		"atob": host.atob, "btoa": host.btoa,
	} {
		if err := vm.GlobalObject().DefineDataProperty(
			name, vm.ToValue(helper), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE,
		); err != nil {
			return nil, ErrInvalidInput
		}
	}
	return host, nil
}

// begin admits a call and its primitive string before inspecting its contents.
// No JavaScript coercion, getters, proxy traps, or object exports are permitted.
// Malformed attempts consume call and any already-admitted input allowance.
func (h *utilityHost) begin(call goja.FunctionCall) goja.String {
	h.checkContext()
	if h.calls >= h.policy.MaxCalls {
		h.failLimit()
	}
	h.calls++
	text, ok := call.Argument(0).(goja.String)
	if !ok {
		h.invalidInput()
	}
	if text.Length() > h.policy.MaxInputBytes-h.inputBytes {
		h.failLimit()
	}
	h.inputBytes += text.Length()
	return text
}

// reserveOutput charges an exact result size before allocation. Callers compute
// size without overflow; a later cancellation does not refund this charge.
func (h *utilityHost) reserveOutput(size int) {
	h.checkContext()
	if size < 0 || size > h.policy.MaxOutputBytes-h.outputBytes {
		h.failLimit()
	}
	h.outputBytes += size
}

// checkContext stops cooperative native work; Run selects the original caller
// error or execution timeout after the callback unwinds. A caught utility limit
// cannot admit more host work or become a successful overall evaluation.
func (h *utilityHost) checkContext() {
	if h.err != nil {
		h.failLimit()
	}
	if h.ctx.Err() != nil {
		panic(h.vm.NewTypeError("JavaScript utility execution canceled"))
	}
}

// invalidInput raises only fixed text. Catching malformed data is allowed;
// arguments, decoded bytes, and library decoder messages never enter the error.
func (h *utilityHost) invalidInput() {
	panic(h.vm.NewTypeError("Invalid base64 input"))
}

// failLimit stops authored execution and records its safe cause for Run.
// Interrupt is uncatchable: initialization cannot recover from exhaustion and
// enable HTTP, nor can execute perform more effects after exhausting a budget.
// The fixed TypeError also unwinds a direct native call outside an active JS loop.
func (h *utilityHost) failLimit() {
	h.err = ErrLimitExceeded
	h.vm.Interrupt(ErrLimitExceeded)
	panic(h.vm.NewTypeError("JavaScript utility limit exceeded"))
}
