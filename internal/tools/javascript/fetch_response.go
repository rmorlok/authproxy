package javascript

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/dop251/goja"
	aphttp "github.com/rmorlok/authproxy/internal/apjs/http"
	apjsruntime "github.com/rmorlok/authproxy/internal/apjs/runtime"
)

// responseValue builds native response metadata without reading its body. The
// closure's consumed flag is VM-owned; workers only receive the Go body reader.
// No upstream request, credentials, or reflected Go response enters the VM.
func (l *fetchLoop) responseValue(response *aphttp.Response) (goja.Value, error) {
	object, headers := l.vm.NewObject(), l.vm.NewObject()

	get := func(call goja.FunctionCall) goja.Value {
		name, ok := call.Argument(0).(goja.String)
		if !ok || name.Length() > l.options.MaxRequestBytes {
			return goja.Null()
		}

		values := response.Header.Values(name.String())
		if len(values) == 0 {
			return goja.Null()
		}

		return l.vm.ToValue(strings.Join(values, ", "))
	}

	if err := fetchProperty(headers, "get", l.vm.ToValue(get)); err != nil {
		return nil, err
	}

	for name, value := range map[string]goja.Value{
		"status":  l.vm.ToValue(response.StatusCode),
		"ok":      l.vm.ToValue(response.StatusCode >= 200 && response.StatusCode < 300),
		"headers": headers,
	} {
		if err := fetchProperty(object, name, value); err != nil {
			return nil, err
		}
	}

	consumed := false
	if err := object.DefineAccessorProperty(
		"bodyUsed",
		l.vm.ToValue(
			func(goja.FunctionCall) goja.Value {
				return l.vm.ToValue(consumed)
			},
		),
		goja.Undefined(),
		goja.FLAG_FALSE,
		goja.FLAG_TRUE,
	); err != nil {
		return nil, apjsruntime.ErrEvaluation
	}

	for _, format := range []string{"text", "json", "arrayBuffer"} {
		read := func(goja.FunctionCall) goja.Value {
			if consumed {
				return l.rejected(apjsruntime.ErrInvalidInput)
			}

			// Session readers already enforce stream and aggregate decoded-byte
			// limits. Read and close entirely off the VM goroutine; discard partial
			// bytes on failure, including cancellation and malformed compression.
			body := response.Body
			promise, admitted := l.start(
				func() fetchCompletion {
					defer body.Close()

					data, err := io.ReadAll(body)
					if err != nil {
						data = nil
					}

					return fetchCompletion{body: data, err: err}
				},
				func(done fetchCompletion) (goja.Value, error) {
					if done.err != nil {
						return nil, done.err
					}

					return l.decodeBody(format, done.body)
				},
			)

			// A saturated worker allowance does not disturb an unread body. The
			// caller can await other work and retry, without losing the response.
			consumed = admitted
			return promise
		}

		if err := fetchProperty(object, format, l.vm.ToValue(read)); err != nil {
			return nil, err
		}
	}

	return object, nil
}

// fetchProperty installs a read-only native response field without invoking an
// inherited setter that authored code may have added to Object.prototype.
func fetchProperty(object *goja.Object, name string, value goja.Value) error {
	if err := object.DefineDataProperty(
		name,
		value,
		goja.FLAG_FALSE,
		goja.FLAG_FALSE,
		goja.FLAG_TRUE,
	); err != nil {
		return apjsruntime.ErrEvaluation
	}

	return nil
}

// decodeBody consumes the worker's detached bytes on the VM goroutine. JSON
// parsing uses the captured intrinsic and input depth/node rules, never an
// authored replacement or private parser diagnostics. Text replaces invalid
// UTF-8 sequences; arrayBuffer preserves the bytes exactly.
func (l *fetchLoop) decodeBody(format string, data []byte) (goja.Value, error) {
	switch format {
	case "arrayBuffer":
		return l.vm.ToValue(l.vm.NewArrayBuffer(data)), nil
	case "text":
		return l.vm.ToValue(string(data)), nil
	default:
		// HTTP readers already cap decoded bytes. Each body receives its own
		// traversal allowance, independent of the invocation's initial inputs.
		values, err := l.decoder.Decode(l.ctx, apjsruntime.JSONLimits{
			MaxBytes: len(data),
			MaxDepth: l.limits.MaxDepth,
			MaxNodes: l.limits.MaxNodes,
		}, json.RawMessage(data))
		if err != nil {
			return nil, err
		}

		return values[0], nil
	}
}
