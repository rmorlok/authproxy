package api_key

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/rmorlok/authproxy/internal/database"
	cschema "github.com/rmorlok/authproxy/internal/schema/connectors"
)

// authApplication is the pure-data result of applying a credential to a
// placement. The proxy converts this into gentleman.Request mutations; tests
// inspect it directly without spinning up an HTTP server.
type authApplication struct {
	// Header is the (name, value) pair to set on the outbound request.
	// Empty name means no header should be set.
	HeaderName  string
	HeaderValue string

	// Query is the (name, value) pair to append to the URL's query string.
	// Empty name means no query param should be appended.
	QueryName  string
	QueryValue string
}

// computeAuthApplication returns the auth header / query param to apply to
// the outbound request for the given placement and credential plaintext.
//
// Pure data — no I/O, no mutation. The caller is responsible for translating
// the result into gentleman.Request calls. Returning structured data (rather
// than mutating a gentleman.Request directly) lets unit tests assert on the
// applied auth without booting an HTTP transport.
func computeAuthApplication(placement *cschema.ApiKeyPlacement, plaintext database.ApiKeyCredentialPlaintext) (authApplication, error) {
	if placement == nil {
		return authApplication{}, errors.New("api-key placement is required")
	}
	if plaintext.ApiKey == "" {
		return authApplication{}, errors.New("api-key credential is empty")
	}

	switch placement.Type {
	case cschema.ApiKeyPlacementBearer:
		return authApplication{
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + plaintext.ApiKey,
		}, nil

	case cschema.ApiKeyPlacementHeader:
		if placement.HeaderName == "" {
			return authApplication{}, errors.New("header placement requires header_name")
		}
		return authApplication{
			HeaderName:  placement.HeaderName,
			HeaderValue: placement.Prefix + plaintext.ApiKey,
		}, nil

	case cschema.ApiKeyPlacementQuery:
		if placement.ParamName == "" {
			return authApplication{}, errors.New("query placement requires param_name")
		}
		return authApplication{
			QueryName:  placement.ParamName,
			QueryValue: plaintext.ApiKey,
		}, nil

	case cschema.ApiKeyPlacementBasic:
		if plaintext.Username == "" {
			return authApplication{}, errors.New("basic placement requires a username")
		}
		// Standard HTTP Basic per RFC 7617: base64(userid ":" password).
		encoded := base64.StdEncoding.EncodeToString([]byte(plaintext.Username + ":" + plaintext.ApiKey))
		return authApplication{
			HeaderName:  "Authorization",
			HeaderValue: "Basic " + encoded,
		}, nil

	default:
		return authApplication{}, fmt.Errorf("unsupported api-key placement type %q", placement.Type)
	}
}
