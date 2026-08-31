package namespace

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/util"
	"gopkg.in/yaml.v3"
)

// UnmarshalJSON preserves strict decoding while rejecting an explicit null
// encryptionKeyRef. Omission means no change; clearing a namespace key is a
// separate operation.
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
	if bytes.Equal(bytes.TrimSpace(wire.EncryptionKeyRef), []byte("null")) {
		return fmt.Errorf("encryptionKeyRef must not be null; omit it to leave the key unchanged")
	}

	var reference meta.ObjectReference
	if err := util.DecodeJSONStrict(wire.EncryptionKeyRef, &reference); err != nil {
		return fmt.Errorf("decode encryptionKeyRef: %w", err)
	}
	p.EncryptionKeyRef = &reference
	return nil
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
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "encryptionKeyRef" &&
				node.Content[i+1].Tag == "!!null" {
				return fmt.Errorf("encryptionKeyRef must not be null; omit it to leave the key unchanged")
			}
		}
	}

	*p = NamespaceSpecPatch(decoded)
	return nil
}
