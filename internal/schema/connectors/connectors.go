package connectors

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/schema/common"
)

// Connectors is the top-level definition of connectors in the config file.
//
// Note that the schema for this object is in the parent package.
type Connectors struct {
	AutoMigrate               *bool                 `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`
	AutoMigrationLockDuration *common.HumanDuration `json:"auto_migration_lock_duration,omitempty" yaml:"auto_migration_lock_duration,omitempty"`
	LoadFromList              []Connector           `json:"load_from_list,omitempty" yaml:"load_from_list,omitempty"`

	// IdentifyingLabels defines which label keys differentiate connectors when no explicit ID is specified.
	// Defaults to ["type"] for backwards compatibility.
	IdentifyingLabels []string `json:"identifying_labels,omitempty" yaml:"identifying_labels,omitempty"`
}

func FromList(c []Connector) *Connectors {
	return &Connectors{
		LoadFromList: c,
	}
}

func (c *Connectors) GetAutoMigrate() bool {
	if c.AutoMigrate == nil {
		return true
	}
	return *c.AutoMigrate
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

// GetIdentifyingLabels returns the identifying labels for connector differentiation.
// Defaults to ["type"] for backwards compatibility when not specified.
func (c *Connectors) GetIdentifyingLabels() []string {
	if c == nil || len(c.IdentifyingLabels) == 0 {
		return []string{"type"}
	}
	return c.IdentifyingLabels
}

func (c *Connectors) Validate(vc *common.ValidationContext) error {
	result := &multierror.Error{}
	identifyingLabels := c.GetIdentifyingLabels()

	// Track by identifying label values (JSON-serialized) instead of Type
	identifyingLabelCounts := make(map[string]int)          // serialized labels -> count
	identifyingLabelHasUuidCount := make(map[string]int)    // serialized labels -> count with uuid
	identifyingLabelHasVersionCount := make(map[string]int) // serialized labels -> count with version
	identifyingLabelNoUuidVersionCount := make(map[string]map[uint64]int)

	uuidToCount := make(map[uuid.UUID]int)
	uuidHasVersionsCount := make(map[uuid.UUID]int)
	uuidVersionCount := make(map[uuid.UUID]map[uint64]int)

	// Beyond the validation of the individual connector configurations, this validate method checks to make sure that
	// all connectors in the list are properly differentiated. We allow users to specify connectors only by identifying
	// labels as long as there is only one of them. Assuming unspecified the system auto manages versions, attempting
	// to immediately mark the new version as primary. If the user wants to manage this rollout process directly, they
	// need to specify versions in the config to match what the system is tracking.
	//
	// Likewise, it is possible to have multiple connectors with the same identifying labels to provide alternatives
	// for how to connect. If they want to specify multiple connectors with the same identifying labels, they must
	// explicitly specify UUIDs to differentiate so that the system knows how to manage the upgrade path.

	for i, connector := range c.GetConnectors() {
		// We use a blank validation context at the connector level to account for the future of splitting
		// the connector definition to a separate file.
		if err := connector.Validate(&common.ValidationContext{}); err != nil {
			labelValues := connector.GetIdentifyingLabelValues(identifyingLabels)
			labelKey := serializeLabelValues(labelValues)

			if connector.Id != uuid.Nil && labelKey != "{}" {
				err = multierror.Prefix(err, fmt.Sprintf("connector %s (%s): ", connector.Id.String(), labelKey))
			} else if connector.Id != uuid.Nil {
				err = multierror.Prefix(err, fmt.Sprintf("connector %s: ", connector.Id.String()))
			} else if labelKey != "{}" {
				err = multierror.Prefix(err, fmt.Sprintf("connector with labels %s: ", labelKey))
			} else {
				err = multierror.Prefix(err, fmt.Sprintf("connector %d: ", i))
			}

			result = multierror.Append(result, err)
		}

		// Validate all identifying labels are present
		labelValues := connector.GetIdentifyingLabelValues(identifyingLabels)
		for _, key := range identifyingLabels {
			if _, exists := labelValues[key]; !exists {
				result = multierror.Append(result, vc.NewErrorfForField(
					"load_from_list",
					"connector %d missing required identifying label %q", i, key,
				))
			}
		}

		// Create key from identifying label values
		identifyingKey := serializeLabelValues(labelValues)

		if connector.Id != uuid.Nil {
			uuidToCount[connector.Id]++

			if connector.Version != 0 {
				uuidHasVersionsCount[connector.Id]++
				if uuidVersionCount[connector.Id] == nil {
					uuidVersionCount[connector.Id] = make(map[uint64]int)
				}
				uuidVersionCount[connector.Id][connector.Version]++
			}
		}

		if identifyingKey != "{}" {
			identifyingLabelCounts[identifyingKey]++

			if connector.Id != uuid.Nil {
				identifyingLabelHasUuidCount[identifyingKey]++
			} else {
				if connector.Version != 0 {
					if identifyingLabelNoUuidVersionCount[identifyingKey] == nil {
						identifyingLabelNoUuidVersionCount[identifyingKey] = make(map[uint64]int)
					}
					identifyingLabelNoUuidVersionCount[identifyingKey][connector.Version]++
				}
			}

			if connector.Version != 0 {
				identifyingLabelHasVersionCount[identifyingKey]++
			}
		}
	}

	for key, count := range identifyingLabelCounts {
		if count > 1 && identifyingLabelHasUuidCount[key] < count && identifyingLabelHasVersionCount[key] < count {
			result = multierror.Append(result, vc.NewErrorf("duplicate connectors exist for identifying labels %s without ids or versions specified to fully differentiate", key))
		}
	}

	for id, count := range uuidToCount {
		if count > 1 && count > uuidHasVersionsCount[id] {
			result = multierror.Append(result, vc.NewErrorf("duplicate connectors exist for id %s without differentiated versions", id.String()))
		}
	}

	for key, versionCounts := range identifyingLabelNoUuidVersionCount {
		for version, count := range versionCounts {
			if count > 1 {
				result = multierror.Append(result, vc.NewErrorf("duplicate connectors exist for identifying labels %s with version %d", key, version))
			}
		}
	}

	for id, versionCounts := range uuidVersionCount {
		for version, count := range versionCounts {
			if count > 1 {
				result = multierror.Append(result, vc.NewErrorf("duplicate connectors exist for id %s with version %d", id.String(), version))
			}
		}
	}

	return result.ErrorOrNil()
}

// serializeLabelValues serializes label values to a JSON string for use as a map key.
func serializeLabelValues(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	data, _ := json.Marshal(labels)
	return string(data)
}
