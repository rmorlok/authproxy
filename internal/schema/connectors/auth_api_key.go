package connectors

import (
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// ApiKeyPlacementType identifies how a connector's API key (and optional username for
// basic auth) is applied to outbound proxied requests.
type ApiKeyPlacementType string

const (
	ApiKeyPlacementBearer = ApiKeyPlacementType("bearer")
	ApiKeyPlacementHeader = ApiKeyPlacementType("header")
	ApiKeyPlacementQuery  = ApiKeyPlacementType("query")
	ApiKeyPlacementBasic  = ApiKeyPlacementType("basic")
)

// ApiKeyPlacement declares how the API key is sent on outbound requests. Exactly one
// placement variant must be configured; the runtime injects credentials accordingly.
type ApiKeyPlacement struct {
	// Type selects the placement variant. Required.
	Type ApiKeyPlacementType `json:"type" yaml:"type"`

	// HeaderName is the HTTP header to set when Type == header. Required for header.
	HeaderName string `json:"header_name,omitempty" yaml:"header_name,omitempty"`

	// Prefix is an optional literal prepended to the key value when Type == header.
	// e.g. "Token " produces a header value of "Token <key>".
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"`

	// ParamName is the URL query parameter name when Type == query. Required for query.
	ParamName string `json:"param_name,omitempty" yaml:"param_name,omitempty"`

	// UsernameField is the name of the user-supplied form field that holds the username
	// (or account-id-like value) paired with the api key when Type == basic. Required
	// for basic. The proxy base64-encodes "<username>:<key>" for the Authorization
	// header; the user is never asked for the encoded form directly.
	UsernameField string `json:"username_field,omitempty" yaml:"username_field,omitempty"`
}

func (p *ApiKeyPlacement) Clone() *ApiKeyPlacement {
	if p == nil {
		return nil
	}

	clone := *p
	return &clone
}

// AuthApiKey describes an API-key-authenticated connector.
type AuthApiKey struct {
	Type      AuthType         `json:"type" yaml:"type"`
	Placement *ApiKeyPlacement `json:"placement" yaml:"placement"`
}

func (a *AuthApiKey) GetType() AuthType {
	return AuthTypeAPIKey
}

func (a *AuthApiKey) Clone() AuthImpl {
	if a == nil {
		return nil
	}

	clone := *a
	clone.Placement = a.Placement.Clone()
	return &clone
}

// Validate enforces the api-key schema invariants:
//   - placement is required;
//   - placement.type is one of bearer / header / query / basic;
//   - placement-type-specific required fields are present and well-formed;
//   - fields that don't apply to the chosen placement type are rejected (catches typos).
func (a *AuthApiKey) Validate(vc *common.ValidationContext) error {
	if a == nil {
		return nil
	}

	result := &multierror.Error{}

	if a.Placement == nil {
		result = multierror.Append(result, vc.NewErrorfForField("placement", "is required"))
		return result.ErrorOrNil()
	}

	pvc := vc.PushField("placement")

	switch a.Placement.Type {
	case ApiKeyPlacementBearer:
		// no further fields required
	case ApiKeyPlacementHeader:
		if a.Placement.HeaderName == "" {
			result = multierror.Append(result, pvc.NewErrorfForField("header_name", "is required when placement.type is %q", ApiKeyPlacementHeader))
		} else if !isValidHttpHeaderFieldName(a.Placement.HeaderName) {
			result = multierror.Append(result, pvc.NewErrorfForField("header_name", "%q is not a valid HTTP header name", a.Placement.HeaderName))
		}
	case ApiKeyPlacementQuery:
		if a.Placement.ParamName == "" {
			result = multierror.Append(result, pvc.NewErrorfForField("param_name", "is required when placement.type is %q", ApiKeyPlacementQuery))
		} else if !isValidUrlQueryParamName(a.Placement.ParamName) {
			result = multierror.Append(result, pvc.NewErrorfForField("param_name", "%q is not a valid URL query parameter name", a.Placement.ParamName))
		}
	case ApiKeyPlacementBasic:
		if a.Placement.UsernameField == "" {
			result = multierror.Append(result, pvc.NewErrorfForField("username_field", "is required when placement.type is %q", ApiKeyPlacementBasic))
		}
	case "":
		result = multierror.Append(result, pvc.NewErrorfForField("type", "is required"))
	default:
		result = multierror.Append(result, pvc.NewErrorfForField("type",
			"%q is not a valid api-key placement type; must be one of %q, %q, %q, %q",
			a.Placement.Type,
			ApiKeyPlacementBearer, ApiKeyPlacementHeader, ApiKeyPlacementQuery, ApiKeyPlacementBasic,
		))
	}

	if a.Placement.Type != ApiKeyPlacementHeader {
		if a.Placement.HeaderName != "" {
			result = multierror.Append(result, pvc.NewErrorfForField("header_name", "is only valid when placement.type is %q", ApiKeyPlacementHeader))
		}
		if a.Placement.Prefix != "" {
			result = multierror.Append(result, pvc.NewErrorfForField("prefix", "is only valid when placement.type is %q", ApiKeyPlacementHeader))
		}
	}
	if a.Placement.Type != ApiKeyPlacementQuery && a.Placement.ParamName != "" {
		result = multierror.Append(result, pvc.NewErrorfForField("param_name", "is only valid when placement.type is %q", ApiKeyPlacementQuery))
	}
	if a.Placement.Type != ApiKeyPlacementBasic && a.Placement.UsernameField != "" {
		result = multierror.Append(result, pvc.NewErrorfForField("username_field", "is only valid when placement.type is %q", ApiKeyPlacementBasic))
	}

	return result.ErrorOrNil()
}

var _ AuthImpl = (*AuthApiKey)(nil)
var _ AuthValidator = (*AuthApiKey)(nil)

// isValidHttpHeaderFieldName reports whether s is a valid HTTP field-name per RFC 7230,
// which restricts names to a token: 1*tchar.
func isValidHttpHeaderFieldName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isTchar(s[i]) {
			return false
		}
	}
	return true
}

// isValidUrlQueryParamName uses the same RFC 7230 token rule as headers — conservative
// but covers all common API conventions (alphanumeric plus a small set of symbols) and
// guarantees the value can be emitted into a URL query without escaping.
func isValidUrlQueryParamName(s string) bool {
	return isValidHttpHeaderFieldName(s)
}

// isTchar reports whether c is an RFC 7230 tchar.
func isTchar(c byte) bool {
	switch {
	case c >= '0' && c <= '9':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}
