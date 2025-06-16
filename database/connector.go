package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/sqlh"
	"github.com/rmorlok/authproxy/util"
	"gorm.io/gorm"
	"time"
)

type ConnectorVersionState string

// Value implements the driver.Valuer interface for ConnectorVersionState
func (s ConnectorVersionState) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface for ConnectorVersionState
func (s *ConnectorVersionState) Scan(value interface{}) error {
	if value == nil {
		*s = ""
		return nil
	}

	strVal, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot convert %T to ConnectorVersionState", value)
	}

	*s = ConnectorVersionState(strVal)
	return nil
}

// ConnectorVersionStates is a custom type for a slice of ConnectorVersionState
type ConnectorVersionStates []ConnectorVersionState

// Value implements the driver.Valuer interface for ConnectorVersionStates
func (s ConnectorVersionStates) Value() (driver.Value, error) {
	if len(s) == 0 {
		return nil, nil
	}

	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for ConnectorVersionStates
func (s *ConnectorVersionStates) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}

	switch v := value.(type) {
	case string:
		return json.Unmarshal([]byte(v), s)
	case []byte:
		return json.Unmarshal(v, s)
	default:
		return fmt.Errorf("cannot convert %T to ConnectorVersionStates", value)
	}
}

const (
	// ConnectorVersionStateDraft means the connector definition is being worked on and new users should not connect to
	// this version and existing users should not be upgraded to this version
	ConnectorVersionStateDraft ConnectorVersionState = "draft"

	// ConnectorVersionStatePrimary means that the version has been published and this should be the version used for
	// new connections. Existing connections of this connector type will be upgraded to this version if possible, or
	// transitioned to a state where action is required to complete the upgrade.
	ConnectorVersionStatePrimary ConnectorVersionState = "primary"

	// ConnectorVersionStateActive means that a newer version of the connector has been published, but connections
	// still exist on this version that have not been upgraded.
	ConnectorVersionStateActive ConnectorVersionState = "active"

	// ConnectorVersionStateArchived means that this is an old version of the connect that does not have any active
	// connections running on the version.
	ConnectorVersionStateArchived ConnectorVersionState = "archived"
)

type ConnectorVersion struct {
	ID                  uuid.UUID             `gorm:"column:id;primaryKey"`
	Version             uint64                `gorm:"column:version;primaryKey"`
	State               ConnectorVersionState `gorm:"column:state"`
	Type                string                `gorm:"column:type"`
	Hash                string                `gorm:"column:hash"`
	EncryptedDefinition string                `gorm:"column:encrypted_definition"`
	CreatedAt           time.Time             `gorm:"column:created_at"`
	UpdatedAt           time.Time             `gorm:"column:updated_at"`
	DeletedAt           gorm.DeletedAt        `gorm:"column:deleted_at;index"`
}

func (cv *ConnectorVersion) Validate() error {
	result := &multierror.Error{}

	if cv.ID == uuid.Nil {
		result = multierror.Append(result, errors.New("id is required"))
	}

	if cv.Version == 0 {
		result = multierror.Append(result, errors.New("version is required"))
	}

	switch cv.State {
	case ConnectorVersionStateDraft,
		ConnectorVersionStatePrimary,
		ConnectorVersionStateActive,
		ConnectorVersionStateArchived:
		// Valid state
	default:
		result = multierror.Append(result, errors.New("invalid connector version state"))
	}

	if cv.Hash == "" {
		result = multierror.Append(result, errors.New("hash is required"))
	}

	if cv.Type == "" {
		result = multierror.Append(result, errors.New("type is required"))
	}

	if cv.EncryptedDefinition == "" {
		result = multierror.Append(result, errors.New("encrypted definition is required"))
	}

	return result.ErrorOrNil()
}

// Connector object is returned from queries for connectors, with one record per id. It aggregates some information
// across all versions for a connector.
type Connector struct {
	ConnectorVersion
	TotalVersions int64
	States        ConnectorVersionStates
}

func (db *gormDB) GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error) {
	sess := db.session(ctx)

	var cv ConnectorVersion
	result := sess.First(&cv, "id = ? AND version = ?", id, version)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &cv, nil
}

type UpsertConnectorVersionResult struct {
	ConnectorVersion *ConnectorVersion
	State            ConnectorVersionState
	Version          uint64
}

func (db *gormDB) UpsertConnectorVersion(ctx context.Context, cv *ConnectorVersion) error {
	if cv == nil {
		return errors.New("connector version is nil")
	}

	if validationErr := cv.Validate(); validationErr != nil {
		return validationErr
	}

	if cv.State != ConnectorVersionStateDraft && cv.State != ConnectorVersionStatePrimary {
		return errors.New("can only upsert connector version as draft or primary")
	}

	return db.gorm.Transaction(func(tx *gorm.DB) error {
		sqlDb, err := tx.DB()
		if err != nil {
			return err
		}

		sqb := sq.StatementBuilder.RunWith(sqlDb)

		exitingState, defaultUsed, err := sqlh.ScanWithDefault(sqb.
			Select("state").
			From("connector_versions").
			Where(sq.Eq{"id": cv.ID, "version": cv.Version}).
			QueryRowContext(ctx), ConnectorVersionStateDraft)

		existingRow := !defaultUsed
		if err != nil {
			return err
		}

		if existingRow {
			if exitingState != ConnectorVersionStateDraft {
				return errors.New("cannot modify non-draft connector")
			}

			result, err := sqb.Update("connector_versions").
				Set("state", cv.State).
				Set("type", cv.Type).
				Set("encrypted_definition", cv.EncryptedDefinition).
				Set("updated_at", apctx.GetClock(ctx).Now()).
				Where(sq.Eq{"id": cv.ID, "version": cv.Version}).
				Exec()
			if err != nil {
				return err
			}

			count, err := result.RowsAffected()
			if err != nil {
				return err
			}

			if count != 1 {
				return errors.Errorf("expected to update 1 row for connector version, got %d", count)
			}
		} else {
			// No existing row at this version. Need to verify if there are existing rows, the new version is
			// existing version + 1
			maxVersion := uint64(0)
			err := sqb.
				Select("COALESCE(MAX(version), 0)").
				From("connector_versions").
				Where(sq.Eq{"id": cv.ID}).
				QueryRowContext(ctx).
				Scan(&maxVersion)

			if err != nil {
				return err
			}

			if maxVersion != 0 && maxVersion+1 != cv.Version {
				return errors.New("cannot insert connector version at non-sequential version")
			}

			_, err = sqb.Insert("connector_versions").
				Columns(
					"id",
					"version",
					"state",
					"type",
					"hash",
					"encrypted_definition",
					"created_at",
					"updated_at",
				).
				Values(
					cv.ID,
					cv.Version,
					cv.State,
					cv.Type,
					cv.Hash,
					cv.EncryptedDefinition,
					apctx.GetClock(ctx).Now(),
					apctx.GetClock(ctx).Now(),
				).
				Exec()
			if err != nil {
				return err
			}
		}

		if cv.State == ConnectorVersionStatePrimary {
			// New primary version, update any previous primary to active
			result, err := sqb.Update("connector_versions").
				Set("state", ConnectorVersionStateActive).
				Where(sq.And{
					sq.Eq{"id": cv.ID, "state": ConnectorVersionStatePrimary},
					sq.NotEq{"version": cv.Version},
				}).
				Exec()
			if err != nil {
				return err
			}

			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			db.logger.Debug("updated connector versions from primary to active", "count", count)
		}

		return nil
	})
}

func (db *gormDB) GetConnectorVersionForTypeAndVersion(ctx context.Context, typ string, version uint64) (*ConnectorVersion, error) {
	sess := db.session(ctx)

	var cv ConnectorVersion
	result := sess.Order("created_at DESC").First(&cv, "type = ? AND version = ?", typ, version)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &cv, nil
}

func (db *gormDB) GetConnectorVersionForType(ctx context.Context, typ string) (*ConnectorVersion, error) {
	sess := db.session(ctx)

	var cv ConnectorVersion
	result := sess.Order("created_at DESC").Order("version DESC").First(&cv, "type = ?", typ)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &cv, nil
}

func (db *gormDB) GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state ConnectorVersionState) (*ConnectorVersion, error) {
	sess := db.session(ctx)

	var cv ConnectorVersion
	result := sess.Order("version DESC").First(&cv, "id = ? AND state = ?", id, state)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &cv, nil
}

func (db *gormDB) NewestConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error) {
	sess := db.session(ctx)

	var cv ConnectorVersion
	result := sess.Order("version DESC").First(&cv, "id = ?", id)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &cv, nil
}

func (db *gormDB) NewestPublishedConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error) {
	sess := db.session(ctx)

	var cv ConnectorVersion
	result := sess.Where(`state in ("primary", "active")`).Order("version DESC").First(&cv, "id = ?", id)
	if result.Error != nil {
		if errors.As(result.Error, &gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}

	if result.RowsAffected == 0 {
		return nil, nil
	}

	return &cv, nil
}

type ConnectorOrderByField string

const (
	ConnectorOrderByCreatedAt   ConnectorOrderByField = "created_at"
	ConnectorOrderByUpdatedAt   ConnectorOrderByField = "updated_at"
	ConnectorOrderByDisplayName ConnectorOrderByField = "display_name"
)

type ListConnectorsExecutor interface {
	FetchPage(context.Context) PageResult[Connector]
	Enumerate(context.Context, func(PageResult[Connector]) (keepGoing bool, err error)) error
}

type ListConnectorsBuilder interface {
	ListConnectorsExecutor
	Limit(int32) ListConnectorsBuilder
	ForType(string) ListConnectorsBuilder
	ForConnectorVersionState(ConnectorVersionState) ListConnectorsBuilder
	OrderBy(ConnectorOrderByField, OrderBy) ListConnectorsBuilder
	IncludeDeleted() ListConnectorsBuilder
}

type listConnectorsFilters struct {
	db                *gormDB                 `json:"-"`
	LimitVal          int32                   `json:"limit"`
	Offset            int32                   `json:"offset"`
	StatesVal         []ConnectorVersionState `json:"states,omitempty"`
	TypeVal           []string                `json:"types,omitempty"`
	OrderByFieldVal   *ConnectorOrderByField  `json:"order_by_field"`
	OrderByVal        *OrderBy                `json:"order_by"`
	IncludeDeletedVal bool                    `json:"include_deleted,omitempty"`
}

func (l *listConnectorsFilters) Limit(limit int32) ListConnectorsBuilder {
	l.LimitVal = limit
	return l
}

func (l *listConnectorsFilters) ForConnectorVersionState(state ConnectorVersionState) ListConnectorsBuilder {
	l.StatesVal = []ConnectorVersionState{state}
	return l
}

func (l *listConnectorsFilters) ForType(t string) ListConnectorsBuilder {
	l.TypeVal = []string{t}
	return l
}

func (l *listConnectorsFilters) OrderBy(field ConnectorOrderByField, by OrderBy) ListConnectorsBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listConnectorsFilters) IncludeDeleted() ListConnectorsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectorsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error) {
	db := l.db
	parsed, err := parseCursor[listConnectorsFilters](ctx, db.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db

	return l, nil
}

func (l *listConnectorsFilters) fetchPage(ctx context.Context) PageResult[Connector] {

	q := l.db.session(ctx)

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	// Picks out the row that will be returned as primary based on a ranked priority of the states
	rankedRowsCTE := `
        SELECT
            *,
            ROW_NUMBER() OVER (
                PARTITION BY id
                ORDER BY
                    CASE state
                        WHEN 'primary' THEN 1
                        WHEN 'draft' THEN 2
                        WHEN 'active' THEN 3
                        WHEN 'archived' THEN 4
                        ELSE 5
                    END
            ) AS row_num
        FROM connector_versions
    `

	// Compute aggregate state for the connector across all versions
	connectorVersionCountsCTE := `
        SELECT
            id,
            json_group_array(state) as states,
            count(*) as versions
        FROM connector_versions
        GROUP BY id
    `

	query := sq.Select(`
rr.id,
rr.version,
rr.state,
rr.type,
COALESCE(rr.encrypted_definition, ""),
rr.created_at,
rr.updated_at,
rr.deleted_at,
cvc.states as states, 
cvc.versions as total_versions
`).
		With("ranked_rows", sq.Expr(rankedRowsCTE)).
		With("connector_version_counts", sq.Expr(connectorVersionCountsCTE)).
		From("ranked_rows rr").
		Join("connector_version_counts cvc ON cvc.id = rr.id").
		Where("rr.row_num = ?", 1)

	if len(l.TypeVal) > 0 {
		query = query.Where("rr.type IN ?", l.TypeVal)
	}

	if len(l.StatesVal) > 0 {
		query = query.Where("rr.state IN ?", l.StatesVal)
	}

	if l.IncludeDeletedVal {
		q = q.Unscoped()
	} else {
		query = query.Where("rr.deleted_at IS NULL")
	}

	// Always limit to one more than limit to check if there are more records
	query = query.Limit(uint64(l.LimitVal + 1)).Offset(uint64(l.Offset))

	if l.OrderByFieldVal != nil {
		query = query.OrderBy(fmt.Sprintf("%s %s", *l.OrderByFieldVal, l.OrderByVal.String()))
	}

	sql, args, err := query.ToSql()
	if err != nil {
		// SQL generation should be deterministic
		panic(errors.Errorf("failed to build query: %s", err))
	}

	rows, err := q.Raw(sql, args...).Rows()

	if err != nil {
		return PageResult[Connector]{Error: err}
	}

	var connectors []Connector
	for rows.Next() {
		var c Connector

		// Scan all fields except States
		err := rows.Scan(
			&c.ID,
			&c.Version,
			&c.State,
			&c.Type,
			&c.EncryptedDefinition,
			&c.CreatedAt,
			&c.UpdatedAt,
			&c.DeletedAt,
			&c.States,
			&c.TotalVersions,
		)
		if err != nil {
			return PageResult[Connector]{Error: err}
		}

		connectors = append(connectors, c)
	}

	l.Offset = l.Offset + int32(len(connectors)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := int32(len(connectors)) > l.LimitVal
	if hasMore {
		cursor, err = makeCursor(ctx, l.db.secretKey, l)
		if err != nil {
			return PageResult[Connector]{Error: err}
		}
	}

	return PageResult[Connector]{
		HasMore: hasMore,
		Results: connectors[:util.MinInt32(l.LimitVal, int32(len(connectors)))],
		Cursor:  cursor,
	}
}

func (l *listConnectorsFilters) FetchPage(ctx context.Context) PageResult[Connector] {
	return l.fetchPage(ctx)
}

func (l *listConnectorsFilters) Enumerate(ctx context.Context, callback func(PageResult[Connector]) (keepGoing bool, err error)) error {
	var err error
	keepGoing := true
	hasMore := true

	for err == nil && hasMore && keepGoing {
		result := l.FetchPage(ctx)
		hasMore = result.HasMore

		if result.Error != nil {
			return result.Error
		}
		keepGoing, err = callback(result)
	}

	return err
}

func (db *gormDB) ListConnectorsBuilder() ListConnectorsBuilder {
	return &listConnectorsFilters{
		db:       db,
		LimitVal: 100,
	}
}

func (db *gormDB) ListConnectorsFromCursor(ctx context.Context, cursor string) (ListConnectorsExecutor, error) {
	b := &listConnectorsFilters{
		db:       db,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}
