package connectors

import (
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/stretchr/testify/assert"
)

func TestConnectors_Validate(t *testing.T) {
	// Helper function to create a basic connector
	createConnector := func(id apid.ID, typ string, version uint64) Connector {
		return Connector{
			Id:          id,
			Labels:      map[string]string{"type": typ},
			Version:     version,
			DisplayName: "Test Connector",
			Description: "Test Description",
		}
	}

	// Generate some UUIDs for testing
	id1 := apid.MustParse("cxr_test1111111111aa")
	id2 := apid.MustParse("cxr_test2222222222aa")

	tests := []struct {
		name       string
		connectors *Connectors
		wantErr    bool
		errMsg     string
	}{
		{
			name: "Valid - Single connector",
			connectors: FromList([]Connector{
				createConnector(apid.Nil, "type1", 0),
			}),
			wantErr: false,
		},
		{
			name: "Valid - Multiple connectors with different types",
			connectors: FromList([]Connector{
				createConnector(apid.Nil, "type1", 0),
				createConnector(apid.Nil, "type2", 0),
			}),
			wantErr: false,
		},
		{
			name: "Valid - Multiple connectors with same type but different IDs",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 0),
				createConnector(id2, "type1", 0),
			}),
			wantErr: false,
		},
		{
			name: "Valid - Multiple connectors with same ID but different versions",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 2),
			}),
			wantErr: false,
		},
		{
			name: "Valid - Multiple connectors with same type but different versions (no IDs)",
			connectors: FromList([]Connector{
				createConnector(apid.Nil, "type1", 1),
				createConnector(apid.Nil, "type1", 2),
			}),
			wantErr: false,
		},
		{
			name: "Invalid - Multiple connectors with same type, no IDs, no versions",
			connectors: FromList([]Connector{
				createConnector(apid.Nil, "type1", 0),
				createConnector(apid.Nil, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for identifying labels",
		},
		{
			name: "Invalid - Multiple connectors with same ID, no versions",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 0),
				createConnector(id1, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id cxr_test1111111111aa without differentiated versions",
		},
		{
			name: "Invalid - Multiple connectors with same type and version (no IDs)",
			connectors: FromList([]Connector{
				createConnector(apid.Nil, "type1", 1),
				createConnector(apid.Nil, "type1", 1),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for identifying labels",
		},
		{
			name: "Invalid - Multiple connectors with same ID and version",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 1),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id cxr_test1111111111aa with version 1",
		},
		{
			name: "Invalid - Mixed duplication scenarios",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 1), // Duplicate ID and version
				createConnector(apid.Nil, "type2", 1),
				createConnector(apid.Nil, "type2", 1), // Duplicate type and version (no ID)
				createConnector(id2, "type3", 0),
				createConnector(id2, "type3", 0), // Duplicate ID, no version
			}),
			wantErr: true,
			// Multiple error messages will be returned
		},
		{
			name: "Valid - Mixed valid scenarios",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 2), // Same ID, different version
				createConnector(apid.Nil, "type2", 1),
				createConnector(apid.Nil, "type2", 2), // Same type, different version (no ID)
				createConnector(id2, "type1", 1),      // Same type as first, different ID
			}),
			wantErr: false,
		},
		{
			name: "Invalid - Some connectors with same type have IDs, some don't",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 0),
				createConnector(apid.Nil, "type1", 0),
				createConnector(apid.Nil, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for identifying labels",
		},
		{
			name: "Invalid - Some connectors with same type have IDs, some have versions",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 0),
				createConnector(apid.Nil, "type1", 1),
				createConnector(apid.Nil, "type1", 2),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for identifying labels",
		},
		{
			name: "Invalid - Some connectors with same ID have versions, some don't",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 0),
				createConnector(id1, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id cxr_test1111111111aa without differentiated versions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.connectors.Validate(&common.ValidationContext{})
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestConnectors_Validate_Exhaustive tests all possible combinations of type, ID, and version duplication
func TestConnectors_Validate_Exhaustive(t *testing.T) {
	// Helper function to create a basic connector
	createConnector := func(id apid.ID, typ string, version uint64) Connector {
		return Connector{
			Id:          id,
			Labels:      map[string]string{"type": typ},
			Version:     version,
			DisplayName: "Test Connector",
			Description: "Test Description",
		}
	}

	// Generate some UUIDs for testing
	id1 := apid.MustParse("cxr_test1111111111aa")
	id2 := apid.MustParse("cxr_test2222222222aa")

	// Define all possible combinations for two connectors
	tests := []struct {
		name    string
		conn1   Connector
		conn2   Connector
		wantErr bool
		errMsg  string
	}{
		// Same type, different scenarios
		{
			name:    "Same type, no IDs, no versions",
			conn1:   createConnector(apid.Nil, "type1", 0),
			conn2:   createConnector(apid.Nil, "type1", 0),
			wantErr: true,
			errMsg:  "duplicate connectors exist for identifying labels",
		},
		{
			name:    "Same type, no IDs, different versions",
			conn1:   createConnector(apid.Nil, "type1", 1),
			conn2:   createConnector(apid.Nil, "type1", 2),
			wantErr: false,
		},
		{
			name:    "Same type, no IDs, same versions",
			conn1:   createConnector(apid.Nil, "type1", 1),
			conn2:   createConnector(apid.Nil, "type1", 1),
			wantErr: true,
			errMsg:  "duplicate connectors exist for identifying labels",
		},
		{
			name:    "Same type, different IDs, no versions",
			conn1:   createConnector(id1, "type1", 0),
			conn2:   createConnector(id2, "type1", 0),
			wantErr: false,
		},
		{
			name:    "Same type, different IDs, same versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id2, "type1", 1),
			wantErr: false,
		},
		{
			name:    "Same type, different IDs, different versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id2, "type1", 2),
			wantErr: false,
		},
		{
			name:    "Same type, same IDs, no versions",
			conn1:   createConnector(id1, "type1", 0),
			conn2:   createConnector(id1, "type1", 0),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id cxr_test1111111111aa without differentiated versions",
		},
		{
			name:    "Same type, same IDs, different versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id1, "type1", 2),
			wantErr: false,
		},
		{
			name:    "Same type, same IDs, same versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id1, "type1", 1),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id cxr_test1111111111aa with version 1",
		},

		// Different type, same scenarios
		{
			name:    "Different type, no IDs, no versions",
			conn1:   createConnector(apid.Nil, "type1", 0),
			conn2:   createConnector(apid.Nil, "type2", 0),
			wantErr: false,
		},
		{
			name:    "Different type, no IDs, different versions",
			conn1:   createConnector(apid.Nil, "type1", 1),
			conn2:   createConnector(apid.Nil, "type2", 2),
			wantErr: false,
		},
		{
			name:    "Different type, no IDs, same versions",
			conn1:   createConnector(apid.Nil, "type1", 1),
			conn2:   createConnector(apid.Nil, "type2", 1),
			wantErr: false,
		},
		{
			name:    "Different type, different IDs, no versions",
			conn1:   createConnector(id1, "type1", 0),
			conn2:   createConnector(id2, "type2", 0),
			wantErr: false,
		},
		{
			name:    "Different type, different IDs, same versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id2, "type2", 1),
			wantErr: false,
		},
		{
			name:    "Different type, different IDs, different versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id2, "type2", 2),
			wantErr: false,
		},
		{
			name:    "Different type, same IDs, no versions",
			conn1:   createConnector(id1, "type1", 0),
			conn2:   createConnector(id1, "type2", 0),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id cxr_test1111111111aa without differentiated versions",
		},
		{
			name:    "Different type, same IDs, different versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id1, "type2", 2),
			wantErr: false,
		},
		{
			name:    "Different type, same IDs, same versions",
			conn1:   createConnector(id1, "type1", 1),
			conn2:   createConnector(id1, "type2", 1),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id cxr_test1111111111aa with version 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connectors := FromList([]Connector{tt.conn1, tt.conn2})
			err := connectors.Validate(&common.ValidationContext{})
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
