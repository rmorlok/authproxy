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
	// Other utility globals are not installed by this flag.
	Base64 bool

	// TextCodecs enables UTF-8 TextEncoder and one-shot TextDecoder instances.
	// Constructors and methods share the same allowances as other selected helpers.
	TextCodecs bool

	// MaxCalls counts native helper/method entries and codec initialization,
	// including argument failures that reach them. It also bounds retained
	// native codec instance metadata; JavaScript errors before entry do not count.
	MaxCalls int

	// MaxInputBytes counts string UTF-16 code units and buffer bytes before
	// scanning or allocating. Decoder labels count too; fixed Boolean options
	// do not. Whitespace and invalid strings consume their admitted length.
	MaxInputBytes int

	// MaxOutputBytes bounds reserved output buffer bytes or string UTF-16 code
	// units. Native buffers and Goja string storage use bounded multiples of
	// these caps; instance metadata is bounded by MaxCalls, not a heap quota.
	MaxOutputBytes int
}

// validate rejects unlimited work for enabled helpers. Caps are unused when
// no helpers are selected, so existing zero-valued execution policies still work.
func (p UtilityPolicy) validate() error {
	if (p.Base64 || p.TextCodecs) && (p.MaxCalls <= 0 || p.MaxInputBytes <= 0 || p.MaxOutputBytes <= 0) {
		return ErrInvalidInput
	}
	return nil
}

// bindings lists enabled utility globals for async declaration checks. Their
// installation also protects the properties in pure expression/Run profiles.
func (p UtilityPolicy) bindings() []HostBinding {
	var bindings []HostBinding
	if p.Base64 {
		bindings = append(bindings,
			HostBinding{Name: "atob"},
			HostBinding{Name: "btoa"},
		)
	}
	if p.TextCodecs {
		bindings = append(bindings,
			HostBinding{Name: "TextEncoder"},
			HostBinding{Name: "TextDecoder"},
		)
	}
	return bindings
}

// utilityHost owns one VM's pure-helper accounting and cancellation context.
// Native callbacks run only on the VM owner. Counters never reset or refund
// admitted work; a limit failure interrupts JavaScript and remains sticky for Run.
type utilityHost struct {
	ctx         context.Context // Same deadline used by initialization and execution.
	vm          *goja.Runtime   // Creates native values and fixed, catchable errors.
	policy      UtilityPolicy   // Immutable caller-supplied aggregate allowances.
	calls       int             // Calls admitted before argument validation.
	inputBytes  int             // String code units or buffer bytes admitted before scanning/copying.
	outputBytes int             // Output bytes or string code units reserved before allocation.
	err         error           // Sticky policy failure checked by Run on every exit.
}

// installUtilities binds selected native functions before any trusted adapter
// callback or authored initialization. It exposes no Go objects, I/O, or logger.
// Helpers are non-enumerable, non-writable, and non-configurable in every profile.
func installUtilities(
	ctx context.Context,
	vm *goja.Runtime,
	policy UtilityPolicy,
) (*utilityHost, error) {
	if !policy.Base64 && !policy.TextCodecs {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	host := &utilityHost{
		ctx:    ctx,
		vm:     vm,
		policy: policy,
	}
	if policy.Base64 {
		for name, helper := range map[string]func(goja.FunctionCall) goja.Value{
			"atob": host.atob,
			"btoa": host.btoa,
		} {
			if err := vm.GlobalObject().DefineDataProperty(
				name,
				vm.ToValue(helper),
				goja.FLAG_FALSE, // writable
				goja.FLAG_FALSE, // configurable
				goja.FLAG_FALSE, // enumerable
			); err != nil {
				return nil, ErrInvalidInput
			}
		}
	}
	if policy.TextCodecs {
		if err := newTextCodecs(host).install(); err != nil {
			return nil, err
		}
	}

	return host, nil
}

// begin admits a call and its primitive string before inspecting its contents.
// No JavaScript coercion, getters, proxy traps, or object exports are permitted.
// Malformed attempts consume call and any already-admitted input allowance.
func (h *utilityHost) begin(call goja.FunctionCall) goja.String {
	h.beginCall()
	text, ok := call.Argument(0).(goja.String)
	if !ok {
		h.invalidInput()
	}
	h.consumeInput(text.Length())
	return text
}

// beginCall charges a native helper entry or codec initialization before validating arguments.
func (h *utilityHost) beginCall() {
	h.checkContext()
	if h.calls >= h.policy.MaxCalls {
		h.failLimit()
	}
	h.calls++
}

// consumeInput admits string code units or buffer bytes before processing them.
func (h *utilityHost) consumeInput(size int) {
	if size < 0 || size > h.policy.MaxInputBytes-h.inputBytes {
		h.failLimit()
	}
	h.inputBytes += size
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
