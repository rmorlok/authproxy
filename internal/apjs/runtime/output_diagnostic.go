package runtime

import "errors"

// OutputRejectionReason identifies a fixed cause of unsupported output without
// recording any authored value, property name, path, source, or VM object.
// It supplements ErrInvalidOutput for trusted diagnostics, not public error text.
type OutputRejectionReason string

const (
	OutputUnsupportedValue  OutputRejectionReason = "unsupported-value"  // A non-JSON primitive, undefined, nonfinite number, or absent value.
	OutputUnsupportedObject OutputRejectionReason = "unsupported-object" // A proxy, function, reflected Go value, or unsupported native object.
	OutputCustomPrototype   OutputRejectionReason = "custom-prototype"   // An object or array with a nonstandard, nonnull prototype.
	OutputAccessor          OutputRejectionReason = "accessor"           // An own getter or setter, including a non-enumerable accessor.
	OutputCycle             OutputRejectionReason = "cycle"              // An object revisited along the active conversion path.
	OutputSymbolKey         OutputRejectionReason = "symbol-key"         // An inspected property key that is a symbol.
	OutputArrayShape        OutputRejectionReason = "array-shape"        // A sparse array or one with extra, non-indexed properties.
	OutputInvalidString     OutputRejectionReason = "invalid-string"     // A lone UTF-16 surrogate rejected by typed UTF-8 conversion.
	OutputReflectionFailure OutputRejectionReason = "reflection-failure" // Failed native reflection or missing expected descriptor metadata.
	OutputUnexpectedType    OutputRejectionReason = "unexpected-type"    // A value that does not satisfy an adapter's required root type.
	OutputInvalidShape      OutputRejectionReason = "invalid-shape"      // JSON data that does not satisfy an adapter's required structure.
)

// outputRejectionError retains only a validated constant. Its error chain ends
// at the existing category, so inspecting it cannot recover rejected VM data.
type outputRejectionError struct {
	reason OutputRejectionReason // Closed-set metadata selected by trusted conversion code.
}

// Error preserves the existing public classification without adding the reason.
func (*outputRejectionError) Error() string { return ErrInvalidOutput.Error() }

// Unwrap preserves errors.Is without retaining the original error or VM value.
func (*outputRejectionError) Unwrap() error { return ErrInvalidOutput }

// RejectOutput annotates an unsupported-data failure for trusted adapters that
// narrow the generic JSON contract, such as requiring a Boolean or object root.
// Unknown reasons are discarded and return the bare category: converting an
// authored string to OutputRejectionReason cannot expose it as a diagnostic.
// Execution limits and cancellation must retain their own classifications.
func RejectOutput(reason OutputRejectionReason) error {
	for _, known := range [...]OutputRejectionReason{
		OutputUnsupportedValue,
		OutputUnsupportedObject,
		OutputCustomPrototype,
		OutputAccessor,
		OutputCycle,
		OutputSymbolKey,
		OutputArrayShape,
		OutputInvalidString,
		OutputReflectionFailure,
		OutputUnexpectedType,
		OutputInvalidShape,
	} {
		if reason == known {
			// Retain the canonical constant, not a caller substring that might
			// otherwise keep a much larger private backing string alive.
			return &outputRejectionError{reason: known}
		}
	}

	return ErrInvalidOutput
}

// OutputReason extracts a detached fixed reason through ordinary Go error
// wrappers. Bare categories, execution errors, and limits have no reason.
// Call it on the final evaluation error: cancellation or budget overrides drop
// a preliminary rejection. No logging occurs here, and no path is retained.
func OutputReason(err error) (OutputRejectionReason, bool) {
	var rejected *outputRejectionError
	if !errors.As(err, &rejected) || rejected == nil {
		return "", false
	}
	return rejected.reason, true
}
