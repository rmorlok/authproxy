package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// EncryptedFieldRegistration declares which columns on a table contain encrypted fields
// and how the table resolves to a namespace.
type EncryptedFieldRegistration struct {
	Table          string
	PrimaryKeyCols []string // e.g. ["id"] or ["id", "generation"]
	EncryptedCols  []string // e.g. ["encrypted_access_token", "encrypted_refresh_token"]

	// Direct namespace resolution (most tables)
	NamespaceCol string // e.g. "namespace" — column on this table

	// Indirect namespace resolution via JOIN (e.g. oauth2_tokens → connections)
	JoinTable        string // e.g. "connections"
	JoinLocalCol     string // e.g. "connection_id" — FK column on this table
	JoinRemoteCol    string // e.g. "id" — PK column on join table
	JoinNamespaceCol string // e.g. "namespace" — namespace column on join table

}

func (r EncryptedFieldRegistration) validate() error {
	if r.Table == "" {
		return errors.New("EncryptedFieldRegistration: Table is required")
	}
	if len(r.PrimaryKeyCols) == 0 {
		return errors.New("EncryptedFieldRegistration: at least one PrimaryKeyCols is required")
	}
	if len(r.EncryptedCols) == 0 {
		return errors.New("EncryptedFieldRegistration: at least one EncryptedCols is required")
	}
	hasDirectNS := r.NamespaceCol != ""
	hasJoinNS := r.JoinTable != "" && r.JoinLocalCol != "" && r.JoinRemoteCol != "" && r.JoinNamespaceCol != ""
	if !hasDirectNS && !hasJoinNS {
		return errors.New("EncryptedFieldRegistration: must set either NamespaceCol or all four Join* fields")
	}
	return nil
}

// usesJoin returns true if this registration resolves its namespace via JOIN.
func (r EncryptedFieldRegistration) usesJoin() bool {
	return r.JoinTable != ""
}

// namespaceQualifiedCol returns the fully qualified column reference for the namespace column.
func (r EncryptedFieldRegistration) namespaceQualifiedCol() string {
	if r.usesJoin() {
		return r.JoinTable + "." + r.JoinNamespaceCol
	}
	return r.Table + "." + r.NamespaceCol
}

var (
	registryMu                   sync.Mutex
	encryptedFieldRegistrations  []EncryptedFieldRegistration
	encryptedFieldRegistryLookup map[string]map[string]bool // table -> col -> true
)

// RegisterEncryptedField adds an encrypted field registration to the global registry.
// Panics if the registration is invalid. Must be called during init().
func RegisterEncryptedField(reg EncryptedFieldRegistration) {
	if err := reg.validate(); err != nil {
		panic(err)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	encryptedFieldRegistrations = append(encryptedFieldRegistrations, reg)

	if encryptedFieldRegistryLookup == nil {
		encryptedFieldRegistryLookup = make(map[string]map[string]bool)
	}
	if encryptedFieldRegistryLookup[reg.Table] == nil {
		encryptedFieldRegistryLookup[reg.Table] = make(map[string]bool)
	}
	for _, col := range reg.EncryptedCols {
		encryptedFieldRegistryLookup[reg.Table][col] = true
	}
}

// GetEncryptedFieldRegistrations returns a copy of all registered encrypted field registrations.
func GetEncryptedFieldRegistrations() []EncryptedFieldRegistration {
	registryMu.Lock()
	defer registryMu.Unlock()
	result := make([]EncryptedFieldRegistration, len(encryptedFieldRegistrations))
	copy(result, encryptedFieldRegistrations)
	return result
}

// isRegisteredEncryptedField checks if a table+column pair is in the registry.
func isRegisteredEncryptedField(table, col string) bool {
	registryMu.Lock()
	defer registryMu.Unlock()
	if encryptedFieldRegistryLookup == nil {
		return false
	}
	cols, ok := encryptedFieldRegistryLookup[table]
	if !ok {
		return false
	}
	return cols[col]
}

// ReEncryptionTarget represents one encrypted field on one row that needs re-encryption.
type ReEncryptionTarget struct {
	Table                     string
	PrimaryKeyCols            []string                // column names in PK order (from registration)
	PrimaryKeyValues          []any                   // values in PK column order
	FieldColumn               string                  // which encrypted column
	EncryptedFieldValue       encfield.EncryptedField // current value (contains the active DEK ID)
	TargetDataEncryptionKeyId apid.ID                 // target DEK for the namespace
}

// ReEncryptedFieldUpdate carries a re-encrypted value and the captured value it
// replaces. The update is skipped if the stored value changed in the meantime.
type ReEncryptedFieldUpdate struct {
	Table            string
	PrimaryKeyCols   []string
	PrimaryKeyValues []any
	FieldColumn      string

	// ExpectedValue is the nonzero encrypted value read before re-encryption.
	// Both its key ID and ciphertext must still match when NewValue is written.
	ExpectedValue encfield.EncryptedField
	NewValue      encfield.EncryptedField
}

// reEncryptionPageSize is the number of rows EnumerateFieldsRequiringReEncryption
// reads per page.
const reEncryptionPageSize = 100

func (s *service) EnumerateFieldsRequiringReEncryption(
	ctx context.Context,
	callback func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error),
) error {
	return s.enumerateFieldsRequiringReEncryption(ctx, reEncryptionPageSize, callback)
}

// enumerateFieldsRequiringReEncryption is EnumerateFieldsRequiringReEncryption with
// a configurable page size, so tests can span several pages with few rows.
func (s *service) enumerateFieldsRequiringReEncryption(
	ctx context.Context,
	pageSize uint64,
	callback func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error),
) error {
	regs := GetEncryptedFieldRegistrations()
	if len(regs) == 0 {
		_, err := callback(nil, true)
		return err
	}

	for regIdx, reg := range regs {
		isLastReg := regIdx == len(regs)-1

		// after is the primary key of the last row read from this table, or nil before
		// the first page. Paging by key rather than offset matters here: callers
		// re-encrypt each page before reading the next, which takes those rows out of
		// the query's filter, and an offset would then skip the rows that moved up.
		var after []any

		for {
			targets, totalRows, lastKey, err := s.queryReEncryptionPage(ctx, reg, pageSize, after)
			if err != nil {
				return fmt.Errorf("failed to query re-encryption page for table %s: %w", reg.Table, err)
			}

			lastPageForTable := uint64(totalRows) <= pageSize
			isLastPage := isLastReg && lastPageForTable

			keepGoing, err := callback(targets, isLastPage)
			if err != nil {
				return err
			}

			if keepGoing == pagination.Stop || lastPageForTable {
				break
			}

			after = lastKey
		}
	}

	return nil
}

// queryReEncryptionPage queries one page of rows needing re-encryption for a
// registration, in primary key order, starting after the row whose primary key values
// are after (nil starts at the beginning). It returns the targets found, the number of
// rows the query returned before field-level filtering (up to pageSize+1, so callers
// can tell whether more rows follow), and the primary key values of the last row in
// the page, to pass as after for the next page.
func (s *service) queryReEncryptionPage(
	ctx context.Context,
	reg EncryptedFieldRegistration,
	pageSize uint64,
	after []any,
) ([]ReEncryptionTarget, int, []any, error) {
	// Build select columns: PK cols (qualified) + encrypted cols (qualified) + target DEK ID.
	var selectCols []string
	for _, pk := range reg.PrimaryKeyCols {
		selectCols = append(selectCols, reg.Table+"."+pk)
	}
	for _, ec := range reg.EncryptedCols {
		selectCols = append(selectCols, reg.Table+"."+ec)
	}
	selectCols = append(selectCols, "namespaces.target_data_encryption_key_id")

	// Build base query
	q := s.sq.
		Select(selectCols...).
		From(reg.Table)

	// JOIN to resolve namespace
	if reg.usesJoin() {
		// Join through intermediate table to namespaces
		q = q.Join(fmt.Sprintf(
			"%s ON %s.%s = %s.%s",
			reg.JoinTable,
			reg.Table, reg.JoinLocalCol,
			reg.JoinTable, reg.JoinRemoteCol,
		))
		q = q.Join(fmt.Sprintf(
			"%s ON %s.%s = %s.path",
			NamespacesTable,
			reg.JoinTable, reg.JoinNamespaceCol,
			NamespacesTable,
		))
		// Exclude deleted rows in the join table
		q = q.Where(sq.Eq{reg.JoinTable + ".deleted_at": nil})
	} else {
		q = q.Join(fmt.Sprintf(
			"%s ON %s.%s = %s.path",
			NamespacesTable,
			reg.Table, reg.NamespaceCol,
			NamespacesTable,
		))
	}

	// WHERE conditions
	q = q.Where(sq.Eq{reg.Table + ".deleted_at": nil})
	q = q.Where(sq.NotEq{"namespaces.target_data_encryption_key_id": nil})
	q = q.Where(sq.Eq{NamespacesTable + ".deleted_at": nil})

	// OR condition: at least one encrypted col doesn't match the target
	var orConditions []sq.Sqlizer
	for _, ec := range reg.EncryptedCols {
		qualifiedCol := reg.Table + "." + ec
		var jsonIdExpr string
		if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
			jsonIdExpr = fmt.Sprintf("COALESCE(%s ->> 'id', '')", qualifiedCol)
		} else {
			jsonIdExpr = fmt.Sprintf("COALESCE(json_extract(%s, '$.id'), '')", qualifiedCol)
		}
		orConditions = append(orConditions, sq.Expr(
			fmt.Sprintf("(%s != namespaces.target_data_encryption_key_id AND %s IS NOT NULL)", jsonIdExpr, qualifiedCol),
		))
	}
	if len(orConditions) == 1 {
		q = q.Where(orConditions[0])
	} else {
		q = q.Where(sq.Or(orConditions))
	}

	// Order and paginate by primary key, continuing after the previous page's last row.
	var orderCols []string
	for _, pk := range reg.PrimaryKeyCols {
		orderCols = append(orderCols, reg.Table+"."+pk)
	}
	if len(after) > 0 {
		q = q.Where(primaryKeyAfter(orderCols, after))
	}
	q = q.OrderBy(strings.Join(orderCols, ", ")).
		Limit(pageSize + 1)

	rows, err := q.RunWith(s.db).Query()
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()

	type rowData struct {
		pkValues      []any
		encryptedVals []encfield.EncryptedField
		targetDEKId   apid.ID
	}

	var allRows []rowData
	for rows.Next() {
		rd := rowData{
			pkValues:      make([]any, len(reg.PrimaryKeyCols)),
			encryptedVals: make([]encfield.EncryptedField, len(reg.EncryptedCols)),
		}

		// Build scan destinations
		scanDest := make([]any, 0, len(reg.PrimaryKeyCols)+len(reg.EncryptedCols)+1)
		for i := range reg.PrimaryKeyCols {
			scanDest = append(scanDest, &rd.pkValues[i])
		}
		for i := range reg.EncryptedCols {
			scanDest = append(scanDest, &rd.encryptedVals[i])
		}
		scanDest = append(scanDest, &rd.targetDEKId)

		if err := rows.Scan(scanDest...); err != nil {
			return nil, 0, nil, err
		}
		// Drivers may scan text as []byte. Compared as a parameter, SQLite orders a
		// blob after every text value, so the next page's key must be text.
		for i, v := range rd.pkValues {
			if b, ok := v.([]byte); ok {
				rd.pkValues[i] = string(b)
			}
		}
		allRows = append(allRows, rd)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, err
	}

	totalRows := len(allRows)
	if totalRows > int(pageSize) {
		allRows = allRows[:pageSize]
	}

	var lastKey []any
	if len(allRows) > 0 {
		lastKey = allRows[len(allRows)-1].pkValues
	}

	// Build targets: one per field per row where that field mismatches
	var targets []ReEncryptionTarget
	for _, rd := range allRows {
		for i, ec := range reg.EncryptedCols {
			ef := rd.encryptedVals[i]
			if ef.IsZero() {
				continue
			}
			if ef.ID != rd.targetDEKId {
				targets = append(targets, ReEncryptionTarget{
					Table:                     reg.Table,
					PrimaryKeyCols:            reg.PrimaryKeyCols,
					PrimaryKeyValues:          rd.pkValues,
					FieldColumn:               ec,
					EncryptedFieldValue:       ef,
					TargetDataEncryptionKeyId: rd.targetDEKId,
				})
			}
		}
	}

	return targets, totalRows, lastKey, nil
}

// primaryKeyAfter restricts a query to rows whose primary key sorts after values, in
// the lexicographic order of cols that ORDER BY cols uses. cols are qualified column
// names and values their values for one row, in the same order. A composite key uses
// a row-value comparison, which both SQLite and PostgreSQL support and which compares
// with the same collation as ORDER BY.
func primaryKeyAfter(cols []string, values []any) sq.Sqlizer {
	if len(cols) == 1 {
		return sq.Gt{cols[0]: values[0]}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(values)), ", ")
	return sq.Expr(fmt.Sprintf("(%s) > (%s)", strings.Join(cols, ", "), placeholders), values...)
}

// BatchUpdateReEncryptedFields replaces captured encrypted values only on live
// rows whose key ID and ciphertext still match. Concurrent edits, cleared fields,
// and deleted rows are skipped without changing encrypted_at; a later sweep can
// re-encrypt any current values still using an old key. Updates are independent,
// so a validation or database error may leave earlier updates applied.
func (s *service) BatchUpdateReEncryptedFields(ctx context.Context, updates []ReEncryptedFieldUpdate) error {
	now := apctx.GetClock(ctx).Now()

	for _, u := range updates {
		if !isRegisteredEncryptedField(u.Table, u.FieldColumn) {
			return fmt.Errorf("table %q field %q is not a registered encrypted field", u.Table, u.FieldColumn)
		}

		if len(u.PrimaryKeyCols) != len(u.PrimaryKeyValues) {
			return fmt.Errorf("PrimaryKeyCols and PrimaryKeyValues length mismatch for table %q", u.Table)
		}

		if u.ExpectedValue.IsZero() {
			return fmt.Errorf("expected encrypted value is required for %s.%s", u.Table, u.FieldColumn)
		}

		whereClause := sq.Eq{"deleted_at": nil}
		for i, col := range u.PrimaryKeyCols {
			whereClause[col] = u.PrimaryKeyValues[i]
		}

		// Compare the JSON members, not the serialized object: SQLite preserves
		// whitespace and property order, while PostgreSQL stores normalized JSONB.
		if s.cfg.GetProvider() == config.DatabaseProviderPostgres {
			whereClause[u.FieldColumn+" ->> 'id'"] = u.ExpectedValue.ID
			whereClause[u.FieldColumn+" ->> 'd'"] = u.ExpectedValue.Data
		} else {
			whereClause["json_extract("+u.FieldColumn+", '$.id')"] = u.ExpectedValue.ID
			whereClause["json_extract("+u.FieldColumn+", '$.d')"] = u.ExpectedValue.Data
		}

		_, err := s.sq.
			Update(u.Table).
			Set(u.FieldColumn, u.NewValue).
			Set("encrypted_at", now).
			Where(whereClause).
			RunWith(s.db).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to update re-encrypted field %s.%s: %w", u.Table, u.FieldColumn, err)
		}
	}

	return nil
}
