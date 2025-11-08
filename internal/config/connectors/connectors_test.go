package connectors

import (
	"testing"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/config/common"
	"github.com/stretchr/testify/assert"
)

func TestConnectors_Validate(t *testing.T) {
	// Helper function to create a basic connector
	createConnector := func(id uuid.UUID, typ string, version uint64) Connector {
		return Connector{
			Id:          id,
			Type:        typ,
			Version:     version,
			DisplayName: "Test Connector",
			Description: "Test Description",
		}
	}

	// Generate some UUIDs for testing
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	tests := []struct {
		name       string
		connectors *Connectors
		wantErr    bool
		errMsg     string
	}{
		{
			name: "Valid - Single connector",
			connectors: FromList([]Connector{
				createConnector(uuid.Nil, "type1", 0),
			}),
			wantErr: false,
		},
		{
			name: "Valid - Multiple connectors with different types",
			connectors: FromList([]Connector{
				createConnector(uuid.Nil, "type1", 0),
				createConnector(uuid.Nil, "type2", 0),
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
				createConnector(uuid.Nil, "type1", 1),
				createConnector(uuid.Nil, "type1", 2),
			}),
			wantErr: false,
		},
		{
			name: "Invalid - Multiple connectors with same type, no IDs, no versions",
			connectors: FromList([]Connector{
				createConnector(uuid.Nil, "type1", 0),
				createConnector(uuid.Nil, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for type type1 without ids or versions specified to fully differentiate",
		},
		{
			name: "Invalid - Multiple connectors with same ID, no versions",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 0),
				createConnector(id1, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id 11111111-1111-1111-1111-111111111111 without differentiated versions",
		},
		{
			name: "Invalid - Multiple connectors with same type and version (no IDs)",
			connectors: FromList([]Connector{
				createConnector(uuid.Nil, "type1", 1),
				createConnector(uuid.Nil, "type1", 1),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for type type1 with version 1",
		},
		{
			name: "Invalid - Multiple connectors with same ID and version",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 1),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id 11111111-1111-1111-1111-111111111111 with version 1",
		},
		{
			name: "Invalid - Mixed duplication scenarios",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 1), // Duplicate ID and version
				createConnector(uuid.Nil, "type2", 1),
				createConnector(uuid.Nil, "type2", 1), // Duplicate type and version (no ID)
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
				createConnector(uuid.Nil, "type2", 1),
				createConnector(uuid.Nil, "type2", 2), // Same type, different version (no ID)
				createConnector(id2, "type1", 1),      // Same type as first, different ID
			}),
			wantErr: false,
		},
		{
			name: "Invalid - Some connectors with same type have IDs, some don't",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 0),
				createConnector(uuid.Nil, "type1", 0),
				createConnector(uuid.Nil, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for type type1 without ids or versions specified to fully differentiate",
		},
		{
			name: "Invalid - Some connectors with same type have IDs, some have versions",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 0),
				createConnector(uuid.Nil, "type1", 1),
				createConnector(uuid.Nil, "type1", 2),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for type type1 without ids or versions specified to fully differentiate",
		},
		{
			name: "Invalid - Some connectors with same ID have versions, some don't",
			connectors: FromList([]Connector{
				createConnector(id1, "type1", 1),
				createConnector(id1, "type1", 0),
				createConnector(id1, "type1", 0),
			}),
			wantErr: true,
			errMsg:  "duplicate connectors exist for id 11111111-1111-1111-1111-111111111111 without differentiated versions",
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
	createConnector := func(id uuid.UUID, typ string, version uint64) Connector {
		return Connector{
			Id:          id,
			Type:        typ,
			Version:     version,
			DisplayName: "Test Connector",
			Description: "Test Description",
		}
	}

	// Generate some UUIDs for testing
	id1 := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	id2 := uuid.MustParse("22222222-2222-2222-2222-222222222222")

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
			conn1:   createConnector(uuid.Nil, "type1", 0),
			conn2:   createConnector(uuid.Nil, "type1", 0),
			wantErr: true,
			errMsg:  "duplicate connectors exist for type type1 without ids or versions specified to fully differentiate",
		},
		{
			name:    "Same type, no IDs, different versions",
			conn1:   createConnector(uuid.Nil, "type1", 1),
			conn2:   createConnector(uuid.Nil, "type1", 2),
			wantErr: false,
		},
		{
			name:    "Same type, no IDs, same versions",
			conn1:   createConnector(uuid.Nil, "type1", 1),
			conn2:   createConnector(uuid.Nil, "type1", 1),
			wantErr: true,
			errMsg:  "duplicate connectors exist for type type1 with version 1",
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
			errMsg:  "duplicate connectors exist for id 11111111-1111-1111-1111-111111111111 without differentiated versions",
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
			errMsg:  "duplicate connectors exist for id 11111111-1111-1111-1111-111111111111 with version 1",
		},

		// Different type, same scenarios
		{
			name:    "Different type, no IDs, no versions",
			conn1:   createConnector(uuid.Nil, "type1", 0),
			conn2:   createConnector(uuid.Nil, "type2", 0),
			wantErr: false,
		},
		{
			name:    "Different type, no IDs, different versions",
			conn1:   createConnector(uuid.Nil, "type1", 1),
			conn2:   createConnector(uuid.Nil, "type2", 2),
			wantErr: false,
		},
		{
			name:    "Different type, no IDs, same versions",
			conn1:   createConnector(uuid.Nil, "type1", 1),
			conn2:   createConnector(uuid.Nil, "type2", 1),
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
			errMsg:  "duplicate connectors exist for id 11111111-1111-1111-1111-111111111111 without differentiated versions",
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
			errMsg:  "duplicate connectors exist for id 11111111-1111-1111-1111-111111111111 with version 1",
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
