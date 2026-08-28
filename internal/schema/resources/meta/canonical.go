package meta

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

// CanonicalSpecJSON serializes a spec into deterministic JSON. It normalizes
// nested maps and json.RawMessage values by decoding once with UseNumber and
// re-encoding with sorted object keys.
func CanonicalSpecJSON(spec any) ([]byte, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, fmt.Errorf("decode marshaled spec: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode marshaled spec: unexpected additional JSON value")
		}
		return nil, fmt.Errorf("decode marshaled spec: %w", err)
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical spec: %w", err)
	}
	return canonical, nil
}

// SemanticSpecHash returns the lowercase SHA-256 hex digest of canonical spec
// JSON. Metadata and status affect the result only if a caller incorrectly
// includes them in the value passed as spec.
func SemanticSpecHash(spec any) (string, error) {
	canonical, err := CanonicalSpecJSON(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
