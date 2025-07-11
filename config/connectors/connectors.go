package connectors

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/config/common"
	"time"
)

type Connectors struct {
	AutoMigrate               *bool                 `json:"auto_migrate,omitempty" yaml:"auto_migrate,omitempty"`
	AutoMigrationLockDuration *common.HumanDuration `json:"auto_migration_lock_duration,omitempty" yaml:"auto_migration_lock_duration,omitempty"`
	LoadFromList              []Connector           `json:"load_from_list,omitempty" yaml:"load_from_list,omitempty"`
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

func (c *Connectors) Validate() error {
	result := &multierror.Error{}
	typeCount := make(map[string]int)
	typeToHasUuidCount := make(map[string]int)
	typeToHasVersionCount := make(map[string]int)
	typeNoUuidVersionCount := make(map[string]map[uint64]int)
	uuidToCount := make(map[uuid.UUID]int)
	uuidHasVersionsCount := make(map[uuid.UUID]int)
	uuidVersionCount := make(map[uuid.UUID]map[uint64]int)

	// Beyond the validation of the individual connector configurations, this validate method checks to make sure that
	// all connectors in the list are properly differentiated. We allow users to specify connectors only by type as long
	// as there is only one of them. Assuming unspecified the system auto manages versions, attempting to immediately
	// mark the new version as primary. If the user wants to manage this rollout process directly, they need to specify
	// versions in the config to match what the system is tracking.
	//
	// Likewise, it is possible to have multiple connectors of the same type to provide alteratives for how to connect.
	// If they want to specify multiple connectors of the same type, they must explicitly specify UUIDs to differentiate
	// so that the system knows how to manage the upgrade path.

	for i, connector := range c.GetConnectors() {
		if err := connector.Validate(); err != nil {
			if connector.Id != uuid.Nil && connector.Type != "" {
				err = multierror.Prefix(err, fmt.Sprintf("connector %s (%s): ", connector.Id.String(), connector.Type))
			} else if connector.Id != uuid.Nil {
				err = multierror.Prefix(err, fmt.Sprintf("connector %s: ", connector.Id.String()))
			} else if connector.Type != "" {
				err = multierror.Prefix(err, fmt.Sprintf("connector type %s: ", connector.Type))
			} else {
				err = multierror.Prefix(err, fmt.Sprintf("connector %d: ", i))
			}

			result = multierror.Append(result, err)
		}

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

		if connector.Type != "" {
			typeCount[connector.Type]++

			if connector.Id != uuid.Nil {
				typeToHasUuidCount[connector.Type]++
			} else {
				if connector.Version != 0 {
					if typeNoUuidVersionCount[connector.Type] == nil {
						typeNoUuidVersionCount[connector.Type] = make(map[uint64]int)
					}
					typeNoUuidVersionCount[connector.Type][connector.Version]++
				}
			}

			if connector.Version != 0 {
				typeToHasVersionCount[connector.Type]++
			}
		}
	}

	for typ, count := range typeCount {
		if count > 1 && typeToHasUuidCount[typ] < count && typeToHasVersionCount[typ] < count {
			result = multierror.Append(result, fmt.Errorf("duplicate connectors exist for type %s without ids or versions specified to fully differentiate", typ))
		}
	}

	for id, count := range uuidToCount {
		if count > 1 && count > uuidHasVersionsCount[id] {
			result = multierror.Append(result, fmt.Errorf("duplicate connectors exist for id %s without differentiated versions", id.String()))
		}
	}

	for typ, versionCounts := range typeNoUuidVersionCount {
		for version, count := range versionCounts {
			if count > 1 {
				result = multierror.Append(result, fmt.Errorf("duplicate connectors exist for type %s with version %d", typ, version))
			}
		}
	}

	for id, versionCounts := range uuidVersionCount {
		for version, count := range versionCounts {
			if count > 1 {
				result = multierror.Append(result, fmt.Errorf("duplicate connectors exist for id %s with version %d", id.String(), version))
			}
		}
	}

	return result.ErrorOrNil()
}
