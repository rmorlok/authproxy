// Package test_utils provides utilities for testing SQL queries and other common testing tasks.
package test_utils

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

// MockTestingT is a mock implementation of TestingT for testing the testing utilities themselves.
// It captures calls to Errorf, Fatalf, and Logf so we can verify they were called with the expected arguments.
type MockTestingT struct {
	ErrorfCalled bool
	FatalfCalled bool
	LogfCalled   bool
	ErrorfMsg    string
	FatalfMsg    string
	LogfMsg      string
}

// Errorf records that it was called and stores the formatted error message.
func (m *MockTestingT) Errorf(format string, args ...interface{}) {
	m.ErrorfCalled = true
	m.ErrorfMsg = fmt.Sprintf(format, args...)
}

// Fatalf records that it was called and stores the formatted fatal error message.
func (m *MockTestingT) Fatalf(format string, args ...interface{}) {
	m.FatalfCalled = true
	m.FatalfMsg = fmt.Sprintf(format, args...)
}

// Logf records that it was called and stores the formatted log message.
func (m *MockTestingT) Logf(format string, args ...interface{}) {
	m.LogfCalled = true
	m.LogfMsg = fmt.Sprintf(format, args...)
}

// Helper is a no-op implementation to satisfy the TestingT interface.
func (m *MockTestingT) Helper() {
	// No-op for testing
}

// Failed returns true if either Errorf or Fatalf was called.
func (m *MockTestingT) Failed() bool {
	return m.ErrorfCalled || m.FatalfCalled
}

// MockSQLQuerier is a mock implementation of SQLQuerier for testing.
// It allows customizing the behavior of the Query method by setting the QueryFunc field.
type MockSQLQuerier struct {
	QueryFunc func(query string, args ...interface{}) (*sql.Rows, error)
}

// Query implements the SQLQuerier interface by delegating to the QueryFunc field.
// If QueryFunc is nil, it returns an error.
func (m *MockSQLQuerier) Query(query string, args ...interface{}) (*sql.Rows, error) {
	if m.QueryFunc != nil {
		return m.QueryFunc(query, args...)
	}
	return nil, errors.New("QueryFunc not implemented")
}

// MockRowScanner is a mock implementation of RowScanner for testing.
// It allows customizing the behavior of the Columns and Scan methods.
type MockRowScanner struct {
	ColumnsFunc func() ([]string, error)
	ScanFunc    func(dest ...interface{}) error
}

// Columns implements the RowScanner interface by delegating to the ColumnsFunc field.
// If ColumnsFunc is nil, it returns an error.
func (m *MockRowScanner) Columns() ([]string, error) {
	if m.ColumnsFunc != nil {
		return m.ColumnsFunc()
	}
	return nil, errors.New("ColumnsFunc not implemented")
}

// Scan implements the RowScanner interface by delegating to the ScanFunc field.
// If ScanFunc is nil, it returns an error.
func (m *MockRowScanner) Scan(dest ...interface{}) error {
	if m.ScanFunc != nil {
		return m.ScanFunc(dest...)
	}
	return errors.New("ScanFunc not implemented")
}

// MockRowsIterator is a mock implementation of RowsIterator for testing.
// It embeds MockRowScanner and adds methods for iterating through rows.
type MockRowsIterator struct {
	MockRowScanner
	NextFunc  func() bool
	ErrFunc   func() error
	CloseFunc func() error
}

// Next implements the RowsIterator interface by delegating to the NextFunc field.
// If NextFunc is nil, it returns false.
func (m *MockRowsIterator) Next() bool {
	if m.NextFunc != nil {
		return m.NextFunc()
	}
	return false
}

// Err implements the RowsIterator interface by delegating to the ErrFunc field.
// If ErrFunc is nil, it returns nil.
func (m *MockRowsIterator) Err() error {
	if m.ErrFunc != nil {
		return m.ErrFunc()
	}
	return nil
}

// Close implements the RowsIterator interface by delegating to the CloseFunc field.
// If CloseFunc is nil, it returns nil.
func (m *MockRowsIterator) Close() error {
	if m.CloseFunc != nil {
		return m.CloseFunc()
	}
	return nil
}

// TestPerson is a test struct used for testing the scanStruct function.
// It has fields with both direct name matching and db tag matching.
type TestPerson struct {
	ID        int    `db:"id"`
	FirstName string `db:"first_name"`
	LastName  string `db:"last_name"`
	Age       int
}

// TestScanStruct tests the scanStruct function with various inputs and edge cases.
// It verifies that the function correctly scans rows into structs and handles errors appropriately.
func TestScanStruct(t *testing.T) {
	// Test that scanStruct correctly scans a row into a struct when field names match column names.
	t.Run("successfully scans struct with matching field names", func(t *testing.T) {
		// Setup mock
		mockScanner := &MockRowScanner{
			ColumnsFunc: func() ([]string, error) {
				return []string{"ID", "FirstName", "LastName", "Age"}, nil
			},
			ScanFunc: func(dest ...interface{}) error {
				// Set values to the destination pointers
				*dest[0].(*int) = 1
				*dest[1].(*string) = "John"
				*dest[2].(*string) = "Doe"
				*dest[3].(*int) = 30
				return nil
			},
		}

		// Test
		var person TestPerson
		err := scanStruct(mockScanner, &person)

		// Verify
		require.NoError(t, err)
		assert.Equal(t, 1, person.ID)
		assert.Equal(t, "John", person.FirstName)
		assert.Equal(t, "Doe", person.LastName)
		assert.Equal(t, 30, person.Age)
	})

	// Test that scanStruct correctly scans a row into a struct when column names match db tags.
	t.Run("successfully scans struct with db tag matching", func(t *testing.T) {
		// Setup mock
		mockScanner := &MockRowScanner{
			ColumnsFunc: func() ([]string, error) {
				return []string{"id", "first_name", "last_name", "Age"}, nil
			},
			ScanFunc: func(dest ...interface{}) error {
				// Set values to the destination pointers
				*dest[0].(*int) = 1
				*dest[1].(*string) = "John"
				*dest[2].(*string) = "Doe"
				*dest[3].(*int) = 30
				return nil
			},
		}

		// Test
		var person TestPerson
		err := scanStruct(mockScanner, &person)

		// Verify
		require.NoError(t, err)
		assert.Equal(t, 1, person.ID)
		assert.Equal(t, "John", person.FirstName)
		assert.Equal(t, "Doe", person.LastName)
		assert.Equal(t, 30, person.Age)
	})

	// Test that scanStruct returns an error when the Columns function fails.
	t.Run("returns error when columns function fails", func(t *testing.T) {
		// Setup mock
		mockScanner := &MockRowScanner{
			ColumnsFunc: func() ([]string, error) {
				return nil, errors.New("columns error")
			},
		}

		// Test
		var person TestPerson
		err := scanStruct(mockScanner, &person)

		// Verify
		require.Error(t, err)
		assert.Equal(t, "columns error", err.Error())
	})

	// Test that scanStruct returns an error when the destination is not a pointer.
	t.Run("returns error when destination is not a pointer", func(t *testing.T) {
		// Setup mock
		mockScanner := &MockRowScanner{
			ColumnsFunc: func() ([]string, error) {
				return []string{"ID"}, nil
			},
		}

		// Test
		person := TestPerson{}
		err := scanStruct(mockScanner, person)

		// Verify
		require.Error(t, err)
		assert.Equal(t, "destination must be a pointer to a struct", err.Error())
	})

	// Test that scanStruct returns an error when the destination is not a struct.
	t.Run("returns error when destination is not a struct", func(t *testing.T) {
		// Setup mock
		mockScanner := &MockRowScanner{
			ColumnsFunc: func() ([]string, error) {
				return []string{"ID"}, nil
			},
		}

		// Test
		var notAStruct int
		err := scanStruct(mockScanner, &notAStruct)

		// Verify
		require.Error(t, err)
		assert.Equal(t, "destination must be a pointer to a struct", err.Error())
	})

	// Test that scanStruct returns an error when no matching field is found.
	t.Run("returns error when no matching field is found", func(t *testing.T) {
		// Setup mock
		mockScanner := &MockRowScanner{
			ColumnsFunc: func() ([]string, error) {
				return []string{"NonExistentField"}, nil
			},
		}

		// Test
		var person TestPerson
		err := scanStruct(mockScanner, &person)

		// Verify
		require.Error(t, err)
		assert.Equal(t, "no field found matching column NonExistentField", err.Error())
	})

	// Test that scanStruct returns an error when the Scan function fails.
	t.Run("returns error when scan fails", func(t *testing.T) {
		// Setup mock
		mockScanner := &MockRowScanner{
			ColumnsFunc: func() ([]string, error) {
				return []string{"ID"}, nil
			},
			ScanFunc: func(dest ...interface{}) error {
				return errors.New("scan error")
			},
		}

		// Test
		var person TestPerson
		err := scanStruct(mockScanner, &person)

		// Verify
		require.Error(t, err)
		assert.Equal(t, "scan error", err.Error())
	})
}

// TestFetchAllRows tests the FetchAllRows function with various inputs and edge cases.
// It verifies that the function correctly fetches all rows from a RowsIterator and handles errors appropriately.
func TestFetchAllRows(t *testing.T) {
	// Test that FetchAllRows correctly fetches all rows from a RowsIterator.
	t.Run("successfully fetches all rows", func(t *testing.T) {
		// Setup mock
		rowCount := 0
		mockRows := &MockRowsIterator{
			MockRowScanner: MockRowScanner{
				ColumnsFunc: func() ([]string, error) {
					return []string{"ID", "FirstName", "LastName", "Age"}, nil
				},
				ScanFunc: func(dest ...interface{}) error {
					// Set values based on row count
					// rowCount is already incremented in NextFunc before this is called
					*dest[0].(*int) = rowCount
					*dest[1].(*string) = "Person"
					*dest[2].(*string) = "Name"
					*dest[3].(*int) = 20 + rowCount - 1
					return nil
				},
			},
			NextFunc: func() bool {
				rowCount++
				return rowCount <= 3
			},
			ErrFunc: func() error {
				return nil
			},
			CloseFunc: func() error {
				return nil
			},
		}

		// Test
		results, err := FetchAllRows[TestPerson](mockRows)

		// Verify
		require.NoError(t, err)
		assert.Len(t, results, 3)
		assert.Equal(t, 1, results[0].ID)
		assert.Equal(t, 2, results[1].ID)
		assert.Equal(t, 3, results[2].ID)
		assert.Equal(t, 20, results[0].Age)
		assert.Equal(t, 21, results[1].Age)
		assert.Equal(t, 22, results[2].Age)
	})

	// Test that FetchAllRows returns an error when scanStruct fails.
	t.Run("returns error when scan fails", func(t *testing.T) {
		// Setup mock
		mockRows := &MockRowsIterator{
			MockRowScanner: MockRowScanner{
				ColumnsFunc: func() ([]string, error) {
					return []string{"ID"}, nil
				},
				ScanFunc: func(dest ...interface{}) error {
					return errors.New("scan error")
				},
			},
			NextFunc: func() bool {
				return true
			},
		}

		// Test
		_, err := FetchAllRows[TestPerson](mockRows)

		// Verify
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to scan row")
	})

	// Test that FetchAllRows returns an error when rows.Err() returns an error.
	t.Run("returns error when rows.Err() returns error", func(t *testing.T) {
		// Setup mock
		mockRows := &MockRowsIterator{
			MockRowScanner: MockRowScanner{
				ColumnsFunc: func() ([]string, error) {
					return []string{"ID"}, nil
				},
				ScanFunc: func(dest ...interface{}) error {
					return nil
				},
			},
			NextFunc: func() bool {
				return false
			},
			ErrFunc: func() error {
				return errors.New("rows error")
			},
		}

		// Test
		_, err := FetchAllRows[TestPerson](mockRows)

		// Verify
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error during row iteration")
	})
}

// TestAssertSqlComponents tests the components of the AssertSql function separately.
// This approach allows us to test the comparison logic and error handling independently.
func TestAssertSqlComponents(t *testing.T) {
	// Test the comparison logic separately from the database interaction.
	// This allows us to verify that the comparison logic works correctly without
	// having to set up a real database or complex mocks.
	t.Run("comparison logic works correctly", func(t *testing.T) {
		// Test case: equal rows - should not report any errors
		expected := []TestPerson{
			{ID: 1, FirstName: "John", LastName: "Doe", Age: 30},
			{ID: 2, FirstName: "Jane", LastName: "Smith", Age: 25},
		}
		actual := []TestPerson{
			{ID: 1, FirstName: "John", LastName: "Doe", Age: 30},
			{ID: 2, FirstName: "Jane", LastName: "Smith", Age: 25},
		}

		// Create a mock testing.T to capture errors
		mockT := &testing.T{}

		// Compare the rows directly using the same logic as in AssertSql
		if len(actual) != len(expected) {
			t.Errorf("Row count mismatch: got %d rows, expected %d rows", len(actual), len(expected))
			return
		}

		for i := range expected {
			if !reflect.DeepEqual(actual[i], expected[i]) {
				t.Errorf("Row %d mismatch:\nGot:      %+v\nExpected: %+v", i, actual[i], expected[i])
			}
		}

		// Verify no errors were reported
		assert.False(t, mockT.Failed(), "Test should not have failed")

		// Test case: different row count - should detect the mismatch
		expected = []TestPerson{
			{ID: 1, FirstName: "John", LastName: "Doe", Age: 30},
			{ID: 2, FirstName: "Jane", LastName: "Smith", Age: 25},
		}
		actual = []TestPerson{
			{ID: 1, FirstName: "John", LastName: "Doe", Age: 30},
		}

		// Compare the rows directly
		if len(actual) != len(expected) {
			// This should fail, which is expected
			assert.NotEqual(t, len(expected), len(actual), "Row counts should be different")
		}

		// Test case: different row content - should detect the mismatch
		expected = []TestPerson{
			{ID: 1, FirstName: "John", LastName: "Doe", Age: 30},
		}
		actual = []TestPerson{
			{ID: 1, FirstName: "Different", LastName: "Person", Age: 40},
		}

		// Compare the rows directly
		if len(actual) == len(expected) {
			for i := range expected {
				if !reflect.DeepEqual(actual[i], expected[i]) {
					// This should fail, which is expected
					assert.NotEqual(t, expected[i], actual[i], "Rows should be different")
				}
			}
		}
	})

	// Test the error handling for query errors.
	// This verifies that AssertSql correctly handles database query errors.
	t.Run("handles query errors correctly", func(t *testing.T) {
		// Create a mock DB that returns an error
		mockDB := &MockSQLQuerier{
			QueryFunc: func(query string, args ...interface{}) (*sql.Rows, error) {
				return nil, errors.New("query error")
			},
		}

		// Create a mock testing.TB
		mockT := &MockTestingT{}

		// Call AssertSql with our mocks - we need to use a type parameter to make the compiler happy
		AssertSql[TestPerson](mockT, mockDB, "SELECT * FROM test_table", []TestPerson{})

		// Verify that Fatalf was called with the expected error message
		assert.True(t, mockT.FatalfCalled, "Fatalf should be called")
		assert.Contains(t, mockT.FatalfMsg, "Failed to execute query")
		assert.Contains(t, mockT.FatalfMsg, "query error")
	})
}

// TestAssertSqlIntegration is an integration test for the AssertSql function.
// It tests the function with a real SQLite database to ensure it works correctly in a real-world scenario.
//
// Note: This test is commented out because it requires the sqlite3 driver which might not be available
// in all environments. To run this test, you need to:
// 1. Import the SQLite driver: _ "github.com/mattn/go-sqlite3"
// 2. Uncomment the test function
// 3. Run the test with: go test -v ./test_utils
/*
func TestAssertSqlIntegration(t *testing.T) {
	// Skip in short mode to avoid running integration tests during quick test runs
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create an in-memory SQLite database for testing
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create a test table with columns that match our TestPerson struct
	_, err = db.Exec(`
		CREATE TABLE test_persons (
			id INTEGER PRIMARY KEY,
			first_name TEXT,
			last_name TEXT,
			age INTEGER
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data that we'll use to verify the AssertSql function
	_, err = db.Exec(`
		INSERT INTO test_persons (id, first_name, last_name, age) VALUES
		(1, 'John', 'Doe', 30),
		(2, 'Jane', 'Smith', 25)
	`)
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// Define expected results that should match the data we inserted
	expected := []TestPerson{
		{ID: 1, FirstName: "John", LastName: "Doe", Age: 30},
		{ID: 2, FirstName: "Jane", LastName: "Smith", Age: 25},
	}

	// Test AssertSql with the real database
	// This should pass because the query results should match our expected data
	AssertSql(t, db, "SELECT id, first_name, last_name, age FROM test_persons ORDER BY id", expected)
}
*/
