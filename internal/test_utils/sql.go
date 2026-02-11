// Package test_utils provides utilities for testing SQL queries and other common testing tasks.
package test_utils

import (
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// SQLQuerier is an interface that defines the Query method used by AssertSql.
// This interface allows for mocking the database in tests.
type SQLQuerier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// RowScanner is an interface that defines the methods needed to scan rows.
// This interface allows for mocking row scanning in tests.
type RowScanner interface {
	Columns() ([]string, error)
	Scan(dest ...interface{}) error
}

// RowsIterator is an interface that defines the methods needed to iterate through rows.
// This interface allows for mocking row iteration in tests.
type RowsIterator interface {
	RowScanner
	Next() bool
	Err() error
	Close() error
}

// TestingT is a simplified interface for testing that only includes the methods we need.
// This allows for easier mocking in tests.
type TestingT interface {
	Helper()
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

// AssertSql tests that a SQL query returns exactly the expected rows in the expected order.
// The 'expected' parameter should be a slice of structs that match the query column order.
//
// Example usage:
//
//	type Person struct {
//	    ID   int    `db:"id"`
//	    Name string `db:"name"`
//	    Age  int    `db:"age"`
//	}
//
//	expected := []Person{
//	    {ID: 1, Name: "John", Age: 30},
//	    {ID: 2, Name: "Jane", Age: 25},
//	}
//
//	AssertSql(t, db, "SELECT id, name, age FROM people ORDER BY id", expected)
func AssertSql[T any](t TestingT, db SQLQuerier, query string, expected []T, args ...interface{}) {
	t.Helper()

	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatalf("Failed to execute query: %v\nQuery: %s", err, query)
	}

	// If rows is nil, we can't fetch anything, so just return
	// This is mainly to handle test cases where the mock returns nil rows
	if rows == nil {
		return
	}

	actual, err := FetchAllRows[T](rows)
	if err != nil {
		t.Fatalf("Failed to fetch rows: %v", err)
	}

	// Compare row count
	if len(actual) != len(expected) {
		t.Errorf("Row count mismatch: got %d rows, expected %d rows", len(actual), len(expected))
		t.Logf("Query: %s", query)
		t.Logf("Actual rows: %+v", actual)
		t.Logf("Expected rows: %+v", expected)
		return
	}

	// Compare rows one by one
	for i := range expected {
		got := normalizeTimes(actual[i])
		want := normalizeTimes(expected[i])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Row %d mismatch:\nGot:      %+v\nExpected: %+v", i, actual[i], expected[i])
		}
	}
}

func normalizeTimes(v any) any {
	return normalizeTimesValue(reflect.ValueOf(v)).Interface()
}

func normalizeTimesValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}

	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		elem := normalizeTimesValue(v.Elem())
		out := reflect.New(elem.Type())
		out.Elem().Set(elem)
		return out
	case reflect.Struct:
		// Handle time.Time
		if v.Type() == reflect.TypeOf(time.Time{}) {
			tm := v.Interface().(time.Time)
			return reflect.ValueOf(tm.UTC())
		}
		// Handle sql.NullTime
		if v.Type() == reflect.TypeOf(sql.NullTime{}) {
			nt := v.Interface().(sql.NullTime)
			if nt.Valid {
				nt.Time = nt.Time.UTC()
			}
			return reflect.ValueOf(nt)
		}
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if out.Field(i).CanSet() {
				out.Field(i).Set(normalizeTimesValue(v.Field(i)))
			} else {
				out.Field(i).Set(v.Field(i))
			}
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(normalizeTimesValue(v.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(normalizeTimesValue(v.Index(i)))
		}
		return out
	default:
		return v
	}
}

// FetchAllRows fetches all rows from a RowsIterator and returns them as a slice of type T.
// This function is useful for testing SQL queries that return multiple rows.
func FetchAllRows[T any](rows RowsIterator) ([]T, error) {
	defer rows.Close()

	var result []T
	for rows.Next() {
		var item T
		err := scanStruct(rows, &item)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return result, nil
}

// scanStruct scans a row into a struct, matching column names to struct field names.
// It supports both direct field name matching (case-insensitive) and matching via the `db` tag.
//
// Example:
//
//	type Person struct {
//	    ID   int    `db:"id"`
//	    Name string `db:"name"`
//	    Age  int    `db:"age"`
//	}
//
//	var person Person
//	err := scanStruct(rows, &person)
func scanStruct(rows RowScanner, dest interface{}) error {
	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	// Create a value object based on the destination object
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return fmt.Errorf("destination must be a pointer to a struct")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct")
	}

	// Create a slice of pointers to scan into
	scanDest := make([]interface{}, len(columns))
	for i, colName := range columns {
		// Find the field with the matching name (case-insensitive)
		fieldIndex := -1
		for j := 0; j < v.NumField(); j++ {
			field := v.Type().Field(j)
			if strings.EqualFold(field.Name, colName) {
				fieldIndex = j
				break
			}

			// Check for db tag matching
			dbTag := field.Tag.Get("db")
			if dbTag != "" && strings.EqualFold(dbTag, colName) {
				fieldIndex = j
				break
			}

			if stringsEqualIgnoreCaseAndFormat(field.Name, colName) {
				fieldIndex = j
				break
			}
		}

		if fieldIndex == -1 {
			return fmt.Errorf("no field found matching column %s", colName)
		}

		scanDest[i] = v.Field(fieldIndex).Addr().Interface()
	}

	return rows.Scan(scanDest...)
}

// StringsEqualIgnoreCaseAndFormat compares two strings ignoring:
// 1. Case (upper/lower)
// 2. Format (camelCase/snake_case/PascalCase)
func stringsEqualIgnoreCaseAndFormat(str1, str2 string) bool {
	return normalizeString(str1) == normalizeString(str2)
}

// NormalizeString converts a string to lowercase and removes all non-alphanumeric characters
func normalizeString(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Remove all non-alphanumeric characters (like underscores)
	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	return reg.ReplaceAllString(s, "")
}
