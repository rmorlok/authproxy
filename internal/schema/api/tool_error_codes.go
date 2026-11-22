package api

// IsReservedToolErrorCode reports whether code identifies an AuthProxy platform
// failure. Producers use it to prevent authored HTTP mappings and JavaScript
// Tool errors from impersonating platform failures. Matching is exact: custom
// codes retain their original casing and spelling.
//
// ToolResultError.Validate accepts both platform and custom codes because the
// public result contract carries both kinds of failure. This helper only
// classifies a code; producers still validate nonblank codes and safe messages.
func IsReservedToolErrorCode(code string) bool {
	switch code {
	case "INPUT_INVALID",
		"CONNECTION_UNAVAILABLE",
		"UPSTREAM_AUTH_FAILED",
		"UPSTREAM_FAILED",
		"UPSTREAM_PROTOCOL_ERROR",
		"UNSUPPORTED_CAPABILITY",
		"EXECUTION_LIMIT_EXCEEDED",
		"OUTPUT_INVALID",
		"OUTPUT_TOO_LARGE",
		"TOOL_REVISION_CHANGED":
		return true
	default:
		return false
	}
}
