package namespace

import (
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// MarshalJSON preserves the difference between an omitted encryptionKeyRef
// and an explicit null used to clear the namespace key.
func (p NamespaceSpecPatch) MarshalJSON() ([]byte, error) {
	if !p.HasEncryptionKeyRef() {
		return []byte("{}"), nil
	}

	return json.Marshal(struct {
		EncryptionKeyRef *meta.ObjectReference `json:"encryptionKeyRef"`
	}{EncryptionKeyRef: p.EncryptionKeyRef})
}

// UnmarshalJSON preserves strict decoding and distinguishes omission from an
// explicit null encryptionKeyRef.
func (p *NamespaceSpecPatch) UnmarshalJSON(data []byte) error {
	var wire struct {
		EncryptionKeyRef json.RawMessage `json:"encryptionKeyRef"`
	}
	if err := util.DecodeJSONStrict(data, &wire); err != nil {
		return err
	}

	*p = NamespaceSpecPatch{}
	if wire.EncryptionKeyRef == nil {
		return nil
	}
	p.encryptionKeyRefPresent = true

	var reference *meta.ObjectReference
	if err := util.DecodeJSONStrict(wire.EncryptionKeyRef, &reference); err != nil {
		return fmt.Errorf("decode encryptionKeyRef: %w", err)
	}
	p.EncryptionKeyRef = reference
	return nil
}

// MarshalYAML preserves the same omission-versus-null distinction as
// MarshalJSON.
func (p NamespaceSpecPatch) MarshalYAML() (any, error) {
	if !p.HasEncryptionKeyRef() {
		return struct{}{}, nil
	}

	return struct {
		EncryptionKeyRef *meta.ObjectReference `yaml:"encryptionKeyRef"`
	}{EncryptionKeyRef: p.EncryptionKeyRef}, nil
}

// UnmarshalYAML applies the same omission-versus-null rules as UnmarshalJSON.
func (p *NamespaceSpecPatch) UnmarshalYAML(value *yaml.Node) error {
	type plain NamespaceSpecPatch
	var decoded plain
	if err := util.DecodeYAMLNodeStrict(value, &decoded); err != nil {
		return err
	}

	node := value
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = node.Content[0]
	}
	presence := false
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "encryptionKeyRef" {
				presence = true
				break
			}
		}
	}

	*p = NamespaceSpecPatch(decoded)
	p.encryptionKeyRefPresent = presence
	return nil
}
