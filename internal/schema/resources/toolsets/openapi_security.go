package toolsets

import (
	"slices"

	"github.com/rmorlok/authproxy/internal/schema/common"
	"gopkg.in/yaml.v3"
)

// OpenAPISecurityConfig maps provider security scheme identifiers to the
// connection receiving the generated tools. It does not supply credentials,
// change their placement, or grant AuthProxy tool permissions. The importer
// must resolve each operation's security requirements and check compatibility
// with that connection before a mapped scheme can satisfy a requirement.
type OpenAPISecurityConfig struct {
	// ConnectionSchemes contains exact keys from OpenAPI Security Requirement
	// Objects, including URI references where the document version permits them.
	// Names are not normalized and list order does not select an alternative.
	// Omission or an empty list maps no schemes; it never disables requirements
	// from the document or suppresses the connection's normal authentication.
	ConnectionSchemes []string `json:"connectionSchemes,omitempty" yaml:"connectionSchemes,omitempty"`
}

// Validate checks nonblank, unique scheme identifiers without interpreting an
// OpenAPI document or inspecting connection credentials. Nil and empty settings
// supply no mappings. Existence, OR/AND requirements, scopes, roles, and actual
// authentication compatibility remain importer and binding checks.
func (s *OpenAPISecurityConfig) Validate(vc *common.ValidationContext) error {
	if s == nil {
		return nil
	}

	return validateMappingStrings(s.ConnectionSchemes, false, validationContext(vc).PushField("connectionSchemes"))
}

// Clone detaches scheme storage while retaining exact strings, list order, and
// nil versus empty lists. Invalid values remain available for later diagnostics.
func (s *OpenAPISecurityConfig) Clone() *OpenAPISecurityConfig {
	if s == nil {
		return nil
	}

	clone := *s
	clone.ConnectionSchemes = slices.Clone(s.ConnectionSchemes)
	return &clone
}

// UnmarshalJSON rejects unknown, noncanonical, and null configuration fields,
// including null list entries. Decoding a replacement clears omitted fields;
// failed decoding leaves the receiver unchanged.
func (s *OpenAPISecurityConfig) UnmarshalJSON(data []byte) error {
	fields, err := decodeStrictObject(data, "OpenAPI security configuration", "connectionSchemes")
	if err != nil {
		return err
	}

	var decoded OpenAPISecurityConfig
	if raw, ok := fields["connectionSchemes"]; ok {
		decoded.ConnectionSchemes, err = decodeMappingStrings(raw, "connectionSchemes")
		if err != nil {
			return err
		}
	}

	*s = decoded
	return nil
}

// UnmarshalYAML resolves aliases and merges before applying the JSON rules so
// null lists and aliased null entries cannot disappear during typed decoding.
func (s *OpenAPISecurityConfig) UnmarshalYAML(node *yaml.Node) error {
	var raw common.RawJSON
	if err := raw.UnmarshalYAML(node); err != nil {
		return err
	}
	return s.UnmarshalJSON(raw)
}
