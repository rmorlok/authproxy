package runtime

import (
	"context"

	"github.com/dop251/goja"
)

// utilityBindings reserves every shipped pure helper in each execution profile.
// Availability is part of the runtime contract, independent of the caller's
// execution budget. I/O and logging remain separate supplied capabilities.
func utilityBindings() []HostBinding {
	return []HostBinding{
		{Name: "atob"}, {Name: "btoa"},
		{Name: "TextEncoder"}, {Name: "TextDecoder"},
		{Name: "URLSearchParams"},
	}
}

// utilityHost binds pure helpers to the owning VM and execution context. It owns
// no counters: every managed operation consumes the context's shared budget.
// Native callbacks run only on the VM owner and retain no state after that VM.
type utilityHost struct {
	ctx context.Context // Shared deadline and VM-free managed-work ledger.
	vm  *goja.Runtime   // Owner-only native values, fixed errors, and interruption.
}

// installUtilities installs all shipped pure helpers before adapter callbacks
// or authored initialization. The context must come from Run/WithExecutionBudget.
// Globals are non-enumerable, non-writable, and non-configurable; none grants I/O.
func installUtilities(ctx context.Context, vm *goja.Runtime) (*utilityHost, error) {
	if ctx == nil || vm == nil || executionBudgetFromContext(ctx) == nil {
		return nil, ErrInvalidInput
	}
	if err := ChargeWork(ctx, 0); err != nil {
		return nil, err
	}
	host := &utilityHost{ctx: ctx, vm: vm}
	for name, helper := range map[string]func(goja.FunctionCall) goja.Value{
		"atob": host.atob, "btoa": host.btoa,
	} {
		if err := vm.GlobalObject().DefineDataProperty(name, vm.ToValue(helper),
			goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE); err != nil {
			return nil, ErrInvalidInput
		}
	}
	if err := newTextCodecs(host).install(); err != nil {
		return nil, err
	}
	if err := newSearchParams(host).install(); err != nil {
		return nil, err
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

// beginCall charges a native helper entry or construction before validating arguments.
func (h *utilityHost) beginCall() {
	h.checkContext()
	if err := ChargeWork(h.ctx, 1); err != nil {
		h.checkContext()
		h.failLimit()
	}
}

// consumeInput admits inspected code units, bytes, or slots as shared work.
func (h *utilityHost) consumeInput(size int) {
	if err := ChargeWork(h.ctx, size); err != nil {
		h.checkContext()
		h.failLimit()
	}
}

// reserveOutput charges storage before allocation. Counters are shared with
// input/result conversion and HTTP work; cancellation never refunds a charge.
func (h *utilityHost) reserveOutput(size int) {
	h.checkContext()
	if err := ReserveAllocation(h.ctx, size); err != nil {
		h.checkContext()
		h.failLimit()
	}
}

// checkContext stops cooperative native work; Run selects the original caller
// error or execution timeout after the callback unwinds. A caught utility limit
// cannot admit more host work or become a successful overall evaluation.
func (h *utilityHost) checkContext() {
	if executionBudgetFromContext(h.ctx).err() != nil {
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
	executionBudgetFromContext(h.ctx).exhaust()
	h.vm.Interrupt(ErrLimitExceeded)
	panic(h.vm.NewTypeError("JavaScript execution limit exceeded"))
}
