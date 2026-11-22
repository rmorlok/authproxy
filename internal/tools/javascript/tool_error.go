package javascript

import (
	"context"
	"strings"

	"github.com/dop251/goja"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
	"github.com/rmorlok/authproxy/internal/schema/api"
)

// rejected recognizes only this VM's explicit ToolError at the rejected entry
// Promise. The shared runtime has already rejected initialization failures and
// unrelated unhandled rejections. Ordinary exceptions remain opaque, even when
// their fields resemble the public error contract.
func (i *toolResultInterpreter) rejected(
	ctx context.Context,
	value goja.Value,
	encoder *apjsruntime.ResultEncoder,
) (*Result, bool, error) {
	branded, err := i.errorHas(goja.Undefined(), value)
	if err != nil {
		return nil, true, apjsruntime.EvaluationError(err)
	}
	if !branded.ToBoolean() {
		return nil, false, nil
	}
	payload, err := i.errorGet(goja.Undefined(), value)
	if err != nil {
		return nil, true, apjsruntime.EvaluationError(err)
	}

	// Code, message, and details share one output allowance, including JSON
	// field names and punctuation. No getters, coercion, or toJSON are invoked.
	encoded, err := encoder.Encode(payload)
	if err != nil {
		return nil, true, err
	}
	if err := apjsruntime.ReserveAllocation(ctx, 2); err != nil {
		return nil, true, err
	}
	failure := &api.ToolResultError{}
	if err := decodeToolPayload(ctx, encoded, failure); err != nil {
		return nil, true, err
	}

	// Only the constructor's private payload is decoded, so authors cannot set
	// Retryable or ExecutionState. Those remain caller-owned. Details is already
	// bounded JSON; only code/message policy needs further validation here.
	if strings.TrimSpace(failure.Code) == "" || strings.TrimSpace(failure.Message) == "" ||
		api.IsReservedToolErrorCode(failure.Code) {
		return nil, true, apjsruntime.RejectOutput(apjsruntime.OutputInvalidShape)
	}
	if err := apjsruntime.ChargeWork(ctx, 0); err != nil {
		return nil, true, err
	}
	return &Result{Error: failure, Content: []api.ToolContentBlock{}}, true, nil
}
