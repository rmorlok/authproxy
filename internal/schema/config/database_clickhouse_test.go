package config

import (
	"context"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/go-faster/errors"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStringValueType struct {
	value string
	err   error
}

func (m *mockStringValueType) Clone() common.StringValueType {
	return m
}

func (m *mockStringValueType) HasValue(ctx context.Context) bool {
	return true
}

func (m *mockStringValueType) GetValue(ctx context.Context) (string, error) {
	return m.value, m.err
}

func TestDatabaseClickhouse_GetAddresses(t *testing.T) {
	tests := []struct {
		name      string
		db        *DatabaseClickhouse
		expect    []string
		expectErr string
	}{
		{
			name: "addresses take precedence",
			db: &DatabaseClickhouse{
				Addresses:   []string{"addr1", "addr2"},
				AddressList: NewStringValueDirect("addr3,addr4"),
				Address:     NewStringValueDirect("addr5"),
			},
			expect: []string{"addr1", "addr2"},
		},
		{
			name: "address list",
			db: &DatabaseClickhouse{
				AddressList: NewStringValueDirect("addr1,addr2"),
			},
			expect: []string{"addr1", "addr2"},
		},
		{
			name: "single address list",
			db: &DatabaseClickhouse{
				AddressList: NewStringValueDirect("addr1"),
			},
			expect: []string{"addr1"},
		},
		{
			name: "address list take precedence over address",
			db: &DatabaseClickhouse{
				AddressList: NewStringValueDirect("addr1,addr2"),
				Address:     NewStringValueDirect("addr3"),
			},
			expect: []string{"addr1", "addr2"},
		},
		{
			name: "address",
			db: &DatabaseClickhouse{
				Address: NewStringValueDirect("addr1"),
			},
			expect: []string{"addr1"},
		},
		{
			name:      "none configured",
			db:        &DatabaseClickhouse{},
			expectErr: "no clickhouse addresses configured",
		},
		{
			name: "address list error",
			db: &DatabaseClickhouse{
				AddressList: &StringValue{
					InnerVal: &mockStringValueType{err: errors.New("fail")},
				},
			},
			expectErr: "failed to get clickhouse address list: fail",
		},
		{
			name: "address error",
			db: &DatabaseClickhouse{
				Address: &StringValue{
					InnerVal: &mockStringValueType{err: errors.New("fail")},
				},
			},
			expectErr: "failed to get clickhouse address: fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addresses, err := tt.db.GetAddresses(t.Context())
			if tt.expectErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectErr)
				assert.Nil(t, addresses)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expect, addresses)
			}
		})
	}
}

func TestDatabaseClickhouse_ToClickhouseOptions(t *testing.T) {
	tests := []struct {
		name             string
		protocol         *string
		expectedProtocol clickhouse.Protocol
	}{
		{
			name:             "default protocol is HTTP",
			protocol:         nil,
			expectedProtocol: clickhouse.HTTP,
		},
		{
			name:             "http protocol",
			protocol:         util.ToPtr("http"),
			expectedProtocol: clickhouse.HTTP,
		},
		{
			name:             "native protocol",
			protocol:         util.ToPtr("native"),
			expectedProtocol: clickhouse.Native,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &DatabaseClickhouse{
				Address:  NewStringValueDirect("localhost:9000"),
				Database: NewStringValueDirect("testdb"),
				Protocol: tt.protocol,
			}

			opts, err := db.ToClickhouseOptions()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedProtocol, opts.Protocol)
			assert.Equal(t, []string{"localhost:9000"}, opts.Addr)
			assert.Equal(t, "testdb", opts.Auth.Database)
		})
	}
}
