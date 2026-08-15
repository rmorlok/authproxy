package connectors

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Connectors is the top-level definition of connectors in the config file.
//
// Note that the schema for this object is in the parent package.
type Connectors struct {
	AutoMigrationLockDuration *common.HumanDuration `json:"autoMigrationLockDuration,omitempty" yaml:"autoMigrationLockDuration,omitempty"`
	LoadFromList              []Connector           `json:"loadFromList,omitempty" yaml:"loadFromList,omitempty"`
}

func FromList(c []Connector) *Connectors {
	return &Connectors{
		LoadFromList: c,
	}
}

func (c *Connectors) GetAutoMigrationLockDurationOrDefault() time.Duration {
	if c.AutoMigrationLockDuration == nil {
		return 1 * time.Minute
	}
	return c.AutoMigrationLockDuration.Duration
}

func (c *Connectors) GetConnectors() []Connector {
	return c.LoadFromList
}

func (c *Connectors) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}
	for i, connector := range c.GetConnectors() {
		// We use a blank validation context at the connector level to account for the future of splitting
		// the connector definition to a separate file.
		if err := connector.Validate(&common.ValidationContext{}); err != nil {
			if connector.Id != apid.Nil {
				err = multierror.Prefix(err, fmt.Sprintf("connector %s: ", connector.Id.String()))
			} else if connector.Name != "" {
				err = multierror.Prefix(err, fmt.Sprintf("connector %s: ", connector.Name))
			} else {
				err = multierror.Prefix(err, fmt.Sprintf("connector %d: ", i))
			}

			result = multierror.Append(result, err)
		}
	}
	result = multierror.Append(result, c.ValidateIdentities(vc))
	return result.ErrorOrNil()
}

// ValidateIdentities validates only the fields used to reconcile configured
// connector entries. Migration uses it for its identity precheck; full
// connector-definition validation remains the responsibility of Validate.
func (c *Connectors) ValidateIdentities(vc *common.ValidationContext) error {
	result := &multierror.Error{}
	type nameKey struct {
		Namespace string
		Name      common.ResourceName
	}
	type logicalKey struct {
		Id        apid.ID
		Namespace string
		Name      common.ResourceName
	}
	type identityDetails struct {
		ids         map[apid.ID]struct{}
		hasNoId     bool
		versions    map[uint64]int
		unversioned map[string]int
	}

	byName := make(map[nameKey]*identityDetails)
	byLogical := make(map[logicalKey]*identityDetails)
	idNamespaces := make(map[apid.ID]map[string]struct{})
	idNames := make(map[apid.ID]map[common.ResourceName]struct{})

	for i, connector := range c.GetConnectors() {
		if !connector.HasId() && !connector.HasName() {
			result = multierror.Append(result, vc.NewErrorfForField(
				"load_from_list",
				"connector %d must specify name when id is omitted", i,
			))
			continue
		}

		name := connector.Name
		if name == "" {
			name = common.ResourceName(connector.Id.String())
		}
		nk := nameKey{Namespace: connector.GetNamespace(), Name: name}
		nameDetails := byName[nk]
		if nameDetails == nil {
			nameDetails = &identityDetails{ids: make(map[apid.ID]struct{}), versions: make(map[uint64]int)}
			byName[nk] = nameDetails
		}
		if connector.HasId() {
			nameDetails.ids[connector.Id] = struct{}{}
		} else {
			nameDetails.hasNoId = true
		}

		lk := logicalKey{Id: connector.Id}
		if !connector.HasId() {
			lk.Namespace = connector.GetNamespace()
			lk.Name = name
		}
		logicalDetails := byLogical[lk]
		if logicalDetails == nil {
			logicalDetails = &identityDetails{versions: make(map[uint64]int), unversioned: make(map[string]int)}
			byLogical[lk] = logicalDetails
		}
		if connector.HasVersion() {
			logicalDetails.versions[connector.Version]++
		} else {
			state := connector.State
			if state == "" {
				state = "primary"
			}
			logicalDetails.unversioned[state]++
		}

		if connector.HasId() {
			if idNamespaces[connector.Id] == nil {
				idNamespaces[connector.Id] = make(map[string]struct{})
			}
			idNamespaces[connector.Id][connector.GetNamespace()] = struct{}{}
			if connector.HasName() {
				if idNames[connector.Id] == nil {
					idNames[connector.Id] = make(map[common.ResourceName]struct{})
				}
				idNames[connector.Id][connector.Name] = struct{}{}
			}
		}
	}

	for key, details := range byName {
		if len(details.ids) > 1 {
			result = multierror.Append(result, vc.NewErrorf("connector name %q in namespace %q is assigned to multiple ids", key.Name, key.Namespace))
		}
		if details.hasNoId && len(details.ids) > 0 {
			result = multierror.Append(result, vc.NewErrorf("connector name %q in namespace %q mixes entries with and without ids", key.Name, key.Namespace))
		}
	}

	for id, namespaces := range idNamespaces {
		if len(namespaces) > 1 {
			result = multierror.Append(result, vc.NewErrorf("connector %s is assigned to multiple namespaces", id))
		}
	}
	for id, names := range idNames {
		if len(names) > 1 {
			result = multierror.Append(result, vc.NewErrorf("connector %s is assigned multiple names", id))
		}
	}

	for key, details := range byLogical {
		if len(details.unversioned) > 0 && len(details.versions) > 0 {
			if key.Id != apid.Nil {
				result = multierror.Append(result, vc.NewErrorf("connector %s has multiple entries without differentiated versions", key.Id))
			} else {
				result = multierror.Append(result, vc.NewErrorf("connector name %q in namespace %q has multiple entries without differentiated versions", key.Name, key.Namespace))
			}
		}
		for state, count := range details.unversioned {
			if count <= 1 {
				continue
			}
			if key.Id != apid.Nil {
				result = multierror.Append(result, vc.NewErrorf("connector %s has multiple unversioned entries for state %q", key.Id, state))
			} else {
				result = multierror.Append(result, vc.NewErrorf("connector name %q in namespace %q has multiple unversioned entries for state %q", key.Name, key.Namespace, state))
			}
		}
		for version, count := range details.versions {
			if count > 1 {
				if key.Id != apid.Nil {
					result = multierror.Append(result, vc.NewErrorf("duplicate connectors exist for id %s with version %d", key.Id, version))
				} else {
					result = multierror.Append(result, vc.NewErrorf("duplicate connectors exist for name %q in namespace %q with version %d", key.Name, key.Namespace, version))
				}
			}
		}
	}

	return result.ErrorOrNil()
}
