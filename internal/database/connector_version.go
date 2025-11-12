package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/sqlh"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"gorm.io/gorm"
)

type ConnectorVersionState string

type ConnectorVersionId struct {
	Id      uuid.UUID
	Version uint64
}

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

func (db *gormDB) GetConnectorVersions(
	ctx context.Context,
	requested []ConnectorVersionId,
) (map[ConnectorVersionId]*ConnectorVersion, error) {
	if len(requested) == 0 {
		return nil, nil
	}

	ids := make(map[ConnectorVersionId]struct{}, len(requested))
	for _, id := range requested {
		ids[id] = struct{}{}
	}

	sess := db.session(ctx)

	query := sess
	for i, id := range requested {
		if i == 0 {
			query = query.Where("(id = ? AND version = ?)", id.Id, id.Version)
		} else {
			query = query.Or("(id = ? AND version = ?)", id.Id, id.Version)
		}
	}

	var versions []ConnectorVersion
	result := query.Find(&versions)
	if result.Error != nil {
		return nil, result.Error
	}

	versionMap := make(map[ConnectorVersionId]*ConnectorVersion, len(versions))
	for i := range versions {
		id := ConnectorVersionId{
			Id:      versions[i].ID,
			Version: versions[i].Version,
		}
		if _, exists := ids[id]; exists {
			versionMap[id] = &versions[i]
		}
	}

	return versionMap, nil
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

	logger := aplog.NewBuilder(db.logger).
		WithCtx(ctx).
		WithConnectorId(cv.ID).
		Build()
	logger.Debug("upserting connector version")

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
				logger.Error("cannot modify non-draft connector", "existing_state", exitingState)
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
				logger.Error("expected to update 1 row for connector version", "got", count)
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

type ConnectorVersionOrderByField string

const (
	ConnectorVersionOrderById        ConnectorVersionOrderByField = "id"
	ConnectorVersionOrderByVersion   ConnectorVersionOrderByField = "version"
	ConnectorVersionOrderByState     ConnectorVersionOrderByField = "state"
	ConnectorVersionOrderByCreatedAt ConnectorVersionOrderByField = "created_at"
	ConnectorVersionOrderByUpdatedAt ConnectorVersionOrderByField = "updated_at"
	ConnectorVersionOrderByType      ConnectorVersionOrderByField = "type"
)

func IsValidConnectorVersionOrderByField[T string | ConnectorVersionOrderByField](field T) bool {
	switch ConnectorVersionOrderByField(field) {
	case ConnectorVersionOrderById,
		ConnectorVersionOrderByVersion,
		ConnectorVersionOrderByState,
		ConnectorVersionOrderByCreatedAt,
		ConnectorVersionOrderByUpdatedAt,
		ConnectorVersionOrderByType:
		return true
	default:
		return false
	}
}

type ListConnectorVersionsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ConnectorVersion]
	Enumerate(context.Context, func(pagination.PageResult[ConnectorVersion]) (keepGoing bool, err error)) error
}

type ListConnectorVersionsBuilder interface {
	ListConnectorVersionsExecutor
	Limit(int32) ListConnectorVersionsBuilder
	ForType(string) ListConnectorVersionsBuilder
	ForId(uuid.UUID) ListConnectorVersionsBuilder
	ForVersion(version uint64) ListConnectorVersionsBuilder
	ForConnectorVersionState(ConnectorVersionState) ListConnectorVersionsBuilder
	OrderBy(ConnectorVersionOrderByField, pagination.OrderBy) ListConnectorVersionsBuilder
	IncludeDeleted() ListConnectorVersionsBuilder
}

type listConnectorVersionsFilters struct {
	db                *gormDB                       `json:"-"`
	LimitVal          int32                         `json:"limit"`
	Offset            int32                         `json:"offset"`
	StatesVal         []ConnectorVersionState       `json:"states,omitempty"`
	TypeVal           []string                      `json:"types,omitempty"`
	IdsVal            []uuid.UUID                   `json:"ids,omitempty"`
	VersionsVal       []uint64                      `json:"versions,omitempty"`
	OrderByFieldVal   *ConnectorVersionOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy           `json:"order_by"`
	IncludeDeletedVal bool                          `json:"include_deleted,omitempty"`
}

func (l *listConnectorVersionsFilters) Limit(limit int32) ListConnectorVersionsBuilder {
	l.LimitVal = limit
	return l
}

func (l *listConnectorVersionsFilters) ForConnectorVersionState(state ConnectorVersionState) ListConnectorVersionsBuilder {
	l.StatesVal = []ConnectorVersionState{state}
	return l
}

func (l *listConnectorVersionsFilters) ForType(t string) ListConnectorVersionsBuilder {
	l.TypeVal = []string{t}
	return l
}

func (l *listConnectorVersionsFilters) ForId(id uuid.UUID) ListConnectorVersionsBuilder {
	l.IdsVal = []uuid.UUID{id}
	return l
}

func (l *listConnectorVersionsFilters) ForVersion(version uint64) ListConnectorVersionsBuilder {
	l.VersionsVal = []uint64{version}
	return l
}

func (l *listConnectorVersionsFilters) OrderBy(field ConnectorVersionOrderByField, by pagination.OrderBy) ListConnectorVersionsBuilder {
	l.OrderByFieldVal = &field
	l.OrderByVal = &by
	return l
}

func (l *listConnectorVersionsFilters) IncludeDeleted() ListConnectorVersionsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectorVersionsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error) {
	db := l.db
	parsed, err := pagination.ParseCursor[listConnectorVersionsFilters](ctx, db.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.db = db

	return l, nil
}

func (l *listConnectorVersionsFilters) fetchPage(ctx context.Context) pagination.PageResult[ConnectorVersion] {
	q := l.db.session(ctx)

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	query := sq.Select(`
cv.id as id,
cv.version as version,
cv.state as state,
cv.type as type,
cv.encrypted_definition,
cv.created_at as created_at,
cv.updated_at as updated_at,
cv.deleted_at as deleted_at`).
		From("connector_versions cv")

	if len(l.TypeVal) > 0 {
		query = query.Where("cv.type IN ?", l.TypeVal)
	}

	if len(l.IdsVal) > 0 {
		query = query.Where("cv.id IN ?", l.IdsVal)
	}

	if len(l.VersionsVal) > 0 {
		query = query.Where("cv.version IN ?", l.VersionsVal)
	}

	if len(l.StatesVal) > 0 {
		query = query.Where("cv.state IN ?", l.StatesVal)
	}

	if l.IncludeDeletedVal {
		q = q.Unscoped()
	} else {
		query = query.Where("cv.deleted_at IS NULL")
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
		return pagination.PageResult[ConnectorVersion]{Error: err}
	}

	var connectorVersions []ConnectorVersion
	for rows.Next() {
		var cv ConnectorVersion

		// Scan all fields except States
		err := rows.Scan(
			&cv.ID,
			&cv.Version,
			&cv.State,
			&cv.Type,
			&cv.EncryptedDefinition,
			&cv.CreatedAt,
			&cv.UpdatedAt,
			&cv.DeletedAt,
		)
		if err != nil {
			return pagination.PageResult[ConnectorVersion]{Error: err}
		}

		connectorVersions = append(connectorVersions, cv)
	}

	l.Offset = l.Offset + int32(len(connectorVersions)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := int32(len(connectorVersions)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.db.secretKey, l)
		if err != nil {
			return pagination.PageResult[ConnectorVersion]{Error: err}
		}
	}

	return pagination.PageResult[ConnectorVersion]{
		HasMore: hasMore,
		Results: connectorVersions[:util.MinInt32(l.LimitVal, int32(len(connectorVersions)))],
		Cursor:  cursor,
	}
}

func (l *listConnectorVersionsFilters) FetchPage(ctx context.Context) pagination.PageResult[ConnectorVersion] {
	return l.fetchPage(ctx)
}

func (l *listConnectorVersionsFilters) Enumerate(ctx context.Context, callback func(pagination.PageResult[ConnectorVersion]) (keepGoing bool, err error)) error {
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

func (db *gormDB) ListConnectorVersionsBuilder() ListConnectorVersionsBuilder {
	return &listConnectorVersionsFilters{
		db:       db,
		LimitVal: 100,
	}
}

func (db *gormDB) ListConnectorVersionsFromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error) {
	b := &listConnectorVersionsFilters{
		db:       db,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}
