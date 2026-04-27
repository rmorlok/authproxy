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
	PrimaryKeyCols []string // e.g. ["id"] or ["id", "version"]
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
	Table                        string
	PrimaryKeyCols               []string                // column names in PK order (from registration)
	PrimaryKeyValues             []any                   // values in PK column order
	FieldColumn                  string                  // which encrypted column
	EncryptedFieldValue          encfield.EncryptedField // current value (contains EKV ID)
	TargetEncryptionKeyVersionId apid.ID                 // what it should be
}

// ReEncryptedFieldUpdate carries the data to update a single encrypted field after re-encryption.
type ReEncryptedFieldUpdate struct {
	Table            string
	PrimaryKeyCols   []string
	PrimaryKeyValues []any
	FieldColumn      string
	NewValue         encfield.EncryptedField
}

func (s *service) EnumerateFieldsRequiringReEncryption(
	ctx context.Context,
	callback func(targets []ReEncryptionTarget, lastPage bool) (keepGoing pagination.KeepGoing, err error),
) error {
	regs := GetEncryptedFieldRegistrations()
	if len(regs) == 0 {
		_, err := callback(nil, true)
		return err
	}

	for regIdx, reg := range regs {
		isLastReg := regIdx == len(regs)-1

		const pageSize = 100
		offset := uint64(0)

		for {
			targets, totalRows, err := s.queryReEncryptionPage(ctx, reg, pageSize, offset)
			if err != nil {
				return fmt.Errorf("failed to query re-encryption page for table %s: %w", reg.Table, err)
			}

			lastPageForTable := totalRows <= pageSize
			isLastPage := isLastReg && lastPageForTable

			keepGoing, err := callback(targets, isLastPage)
			if err != nil {
				return err
			}

			if keepGoing == pagination.Stop || lastPageForTable {
				break
			}

			offset += pageSize
		}
	}

	return nil
}

// queryReEncryptionPage queries one page of rows needing re-encryption for a registration.
// Returns the targets found and the total number of rows returned by the query (before field-level filtering).
func (s *service) queryReEncryptionPage(
	ctx context.Context,
	reg EncryptedFieldRegistration,
	pageSize uint64,
	offset uint64,
) ([]ReEncryptionTarget, int, error) {
	// Build select columns: PK cols (qualified) + encrypted cols (qualified) + target EKV ID
	var selectCols []string
	for _, pk := range reg.PrimaryKeyCols {
		selectCols = append(selectCols, reg.Table+"."+pk)
	}
	for _, ec := range reg.EncryptedCols {
		selectCols = append(selectCols, reg.Table+"."+ec)
	}
	selectCols = append(selectCols, "namespaces.target_encryption_key_version_id")

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
	q = q.Where(sq.NotEq{"namespaces.target_encryption_key_version_id": nil})
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
			fmt.Sprintf("(%s != namespaces.target_encryption_key_version_id AND %s IS NOT NULL)", jsonIdExpr, qualifiedCol),
		))
	}
	if len(orConditions) == 1 {
		q = q.Where(orConditions[0])
	} else {
		q = q.Where(sq.Or(orConditions))
	}

	// Order and paginate
	var orderCols []string
	for _, pk := range reg.PrimaryKeyCols {
		orderCols = append(orderCols, reg.Table+"."+pk)
	}
	q = q.OrderBy(strings.Join(orderCols, ", ")).
		Limit(pageSize + 1).
		Offset(offset)

	rows, err := q.RunWith(s.db).Query()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	type rowData struct {
		pkValues      []any
		encryptedVals []encfield.EncryptedField
		targetEKVId   apid.ID
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
		scanDest = append(scanDest, &rd.targetEKVId)

		if err := rows.Scan(scanDest...); err != nil {
			return nil, 0, err
		}
		allRows = append(allRows, rd)
	}

	totalRows := len(allRows)
	if totalRows > int(pageSize) {
		allRows = allRows[:pageSize]
	}

	// Build targets: one per field per row where that field mismatches
	var targets []ReEncryptionTarget
	for _, rd := range allRows {
		for i, ec := range reg.EncryptedCols {
			ef := rd.encryptedVals[i]
			if ef.IsZero() {
				continue
			}
			if ef.ID != rd.targetEKVId {
				targets = append(targets, ReEncryptionTarget{
					Table:                        reg.Table,
					PrimaryKeyCols:               reg.PrimaryKeyCols,
					PrimaryKeyValues:             rd.pkValues,
					FieldColumn:                  ec,
					EncryptedFieldValue:          ef,
					TargetEncryptionKeyVersionId: rd.targetEKVId,
				})
			}
		}
	}

	return targets, totalRows, nil
}

func (s *service) BatchUpdateReEncryptedFields(ctx context.Context, updates []ReEncryptedFieldUpdate) error {
	now := apctx.GetClock(ctx).Now()

	for _, u := range updates {
		if !isRegisteredEncryptedField(u.Table, u.FieldColumn) {
			return fmt.Errorf("table %q field %q is not a registered encrypted field", u.Table, u.FieldColumn)
		}

		if len(u.PrimaryKeyCols) != len(u.PrimaryKeyValues) {
			return fmt.Errorf("PrimaryKeyCols and PrimaryKeyValues length mismatch for table %q", u.Table)
		}

		whereClause := sq.Eq{}
		for i, col := range u.PrimaryKeyCols {
			whereClause[col] = u.PrimaryKeyValues[i]
		}

		_, err := s.sq.
			Update(u.Table).
			Set(u.FieldColumn, u.NewValue).
			Set("encrypted_at", now).
			Where(whereClause).
			RunWith(s.db).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to update re-encrypted field %s.%s: %w", u.Table, u.FieldColumn, err)
		}
	}

	return nil
}
