package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/schema/common"
	toolsetschema "github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
)

// generatedToolNameSuffixLength is how many hex digits of the identity hash end
// a generated Tool's name. Sixty-four bits keeps accidental collisions unlikely
// in one namespace, and a real one fails the whole inventory on name uniqueness.
const generatedToolNameSuffixLength = 16

// compiledToolTemplate is one explicit template that has passed compilation,
// serialized once so every connection stores byte-identical definition text.
type compiledToolTemplate struct {
	key         string
	name        common.ResourceName
	labels      map[string]string
	annotations map[string]string
	description string
	verbs       []string
	plaintext   []byte
	digest      string
}

// toolSetInstallPlan is a primary generation's inventory, ready to prepare for
// any matching connection. It holds no connection-specific or resolved values.
type toolSetInstallPlan struct {
	generation uint64
	templates  []compiledToolTemplate
}

// errToolSetSourceUnsupported means a primary generation has no installable
// explicit source. Membership can still be recorded; nothing can be installed.
var errToolSetSourceUnsupported = errors.New("toolset source cannot be installed")

// newToolSetInstallPlan recompiles a published generation's templates. They
// compiled at publication, but operator limits may have changed since, and an
// inventory must never hold a definition the current server cannot compile.
func newToolSetInstallPlan(
	generation uint64,
	definition *toolsetschema.ToolSetDefinition,
) (*toolSetInstallPlan, error) {
	if definition.Source.Explicit == nil {
		return nil, errToolSetSourceUnsupported
	}

	plan := &toolSetInstallPlan{generation: generation}
	for i := range definition.Source.Explicit.Tools {
		template := &definition.Source.Explicit.Tools[i]
		field := fmt.Sprintf("explicit.tools[%d].spec", i)
		if err := compileToolDefinition(&template.Spec, field); err != nil {
			return nil, err
		}

		plaintext, err := json.Marshal(&template.Spec)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize toolset template %q: %w", template.Key, err)
		}

		compiled := compiledToolTemplate{
			key:         template.Key,
			description: template.Spec.Description,
			verbs:       template.Spec.Verbs,
			plaintext:   plaintext,
			digest:      toolDefinitionDigest(plaintext),
		}
		if template.Metadata != nil {
			compiled.name = template.Metadata.Name
			compiled.labels = template.Metadata.Labels
			compiled.annotations = template.Metadata.Annotations
		}
		plan.templates = append(plan.templates, compiled)
	}

	return plan, nil
}

// generatedToolName builds a readable name from the ToolSet and template names,
// ended by a hash of the Tool's logical identity (ToolSet, connection, source
// key). The hash keeps names unique within a namespace, where several bindings
// install the same templates. Names are chosen at first install, and replacement
// inventories that omit a name keep the existing one.
func generatedToolName(
	toolSetName common.ResourceName,
	templateName common.ResourceName,
	toolSetId apid.ID,
	connectionId apid.ID,
	sourceKey string,
) common.ResourceName {
	// NUL cannot appear in IDs, so these fields cannot run together ambiguously.
	identity := sha256.Sum256([]byte(toolSetId.String() + "\x00" + connectionId.String() + "\x00" + sourceKey))
	suffix := hex.EncodeToString(identity[:])[:generatedToolNameSuffixLength]

	stem := string(toolSetName)
	if templateName != "" {
		stem += "-" + string(templateName)
	}

	// Truncation can strand a separator, but the suffix still follows it.
	if limit := common.ResourceNameMaxLength - generatedToolNameSuffixLength - 1; len(stem) > limit {
		stem = strings.TrimRight(stem[:limit], "-_.")
	}

	return common.ResourceName(stem + "-" + suffix)
}

// inventoryFor encrypts the plan for one pending binding's connection namespace
// and pairs each definition with its Tool identity. The IDs are provisional
// because storage resolves retained identities by source key. Encryption stays
// outside the database transaction, and definitions keep their symbolic
// configuration references rather than resolved connection values.
func (s *service) inventoryFor(
	ctx context.Context,
	plan *toolSetInstallPlan,
	logical *database.ToolSet,
	binding *database.ToolSetBinding,
	connection *database.Connection,
) (database.ToolSetInventoryInstall, error) {
	install := database.ToolSetInventoryInstall{
		BindingId:                binding.Id,
		ExpectedEpoch:            binding.Epoch,
		ExpectedSelectorRevision: logical.SelectorRevision,
		Generation:               plan.generation,
		Tools:                    make([]database.PreparedToolSetTool, 0, len(plan.templates)),
	}

	for i := range plan.templates {
		template := &plan.templates[i]

		encrypted, err := s.encrypt.EncryptForNamespace(ctx, connection.Namespace, template.plaintext)
		if err != nil {
			return database.ToolSetInventoryInstall{}, fmt.Errorf("failed to encrypt toolset template %q: %w", template.key, err)
		}

		install.Tools = append(install.Tools, database.PreparedToolSetTool{
			Tool: database.Tool{
				Id:           apid.New(apid.PrefixTool),
				Namespace:    connection.Namespace,
				Name:         generatedToolName(logical.Name, template.name, logical.Id, connection.Id, template.key),
				ConnectionId: connection.Id,
				BindingId:    binding.Id,
				SourceKey:    template.key,
				Description:  template.description,
				Verbs:        template.verbs,
				Labels:       database.Labels(maps.Clone(template.labels)),
				Annotations:  database.Annotations(maps.Clone(template.annotations)),
			},
			EncryptedDefinition: encrypted,
			DefinitionDigest:    template.digest,
		})
	}

	return install, nil
}
