package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/hashicorp/go-multierror"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
	"github.com/rmorlok/authproxy/internal/encfield"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ConnectorDefinitionVersionState string

type ConnectorDefinitionVersionId struct {
	Id      apid.ID
	Version uint64
}

// Value implements the driver.Valuer interface for ConnectorDefinitionVersionState
func (s ConnectorDefinitionVersionState) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface for ConnectorDefinitionVersionState
func (s *ConnectorDefinitionVersionState) Scan(value interface{}) error {
	if value == nil {
		*s = ""
		return nil
	}

	strVal, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot convert %T to ConnectorDefinitionVersionState", value)
	}

	*s = ConnectorDefinitionVersionState(strVal)
	return nil
}

const (
	// ConnectorDefinitionVersionStateDraft means the connector definition is being worked on and new users should not connect to
	// this version and existing users should not be upgraded to this version
	ConnectorDefinitionVersionStateDraft ConnectorDefinitionVersionState = "draft"

	// ConnectorDefinitionVersionStatePrimary means that the version has been published and this should be the version used for
	// new connections. Existing connections of this connector will be upgraded to this version if possible, or
	// transitioned to a state where action is required to complete the upgrade.
	ConnectorDefinitionVersionStatePrimary ConnectorDefinitionVersionState = "primary"

	// ConnectorDefinitionVersionStateActive means that a newer version of the connector has been published, but connections
	// still exist on this version that have not been upgraded.
	ConnectorDefinitionVersionStateActive ConnectorDefinitionVersionState = "active"

	// ConnectorDefinitionVersionStateArchived means that this is an old version of the connect that does not have any active
	// connections running on the version.
	ConnectorDefinitionVersionStateArchived ConnectorDefinitionVersionState = "archived"
)

func IsValidConnectorDefinitionVersionState[T string | ConnectorDefinitionVersionState](state T) bool {
	switch ConnectorDefinitionVersionState(state) {
	case ConnectorDefinitionVersionStateDraft,
		ConnectorDefinitionVersionStatePrimary,
		ConnectorDefinitionVersionStateActive,
		ConnectorDefinitionVersionStateArchived:
		return true
	default:
		return false
	}
}

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:            ConnectorDefinitionVersionsTable,
		PrimaryKeyCols:   []string{"id"},
		EncryptedCols:    []string{"encrypted_definition"},
		JoinTable:        ConnectorsTable,
		JoinLocalCol:     "connector_id",
		JoinRemoteCol:    "id",
		JoinNamespaceCol: "namespace",
	})
}

const ConnectorDefinitionVersionsTable = "connector_definition_versions"

// ConnectorDefinitionVersion is the database representation of a single row
// in connector_definition_versions.
type ConnectorDefinitionVersion struct {
	Id                  apid.ID
	ConnectorId         apid.ID
	Version             uint64
	State               ConnectorDefinitionVersionState
	EncryptedDefinition encfield.EncryptedField
	CreatedAt           time.Time
	UpdatedAt           time.Time
	EncryptedAt         *time.Time
	DeletedAt           *time.Time
}

func (cv *ConnectorDefinitionVersion) cols() []string {
	return []string{
		"id",
		"connector_id",
		"version",
		"state",
		"encrypted_definition",
		"created_at",
		"updated_at",
		"encrypted_at",
		"deleted_at",
	}
}

func (cv *ConnectorDefinitionVersion) fields() []any {
	return []any{
		&cv.Id,
		&cv.ConnectorId,
		&cv.Version,
		&cv.State,
		&cv.EncryptedDefinition,
		&cv.CreatedAt,
		&cv.UpdatedAt,
		&cv.EncryptedAt,
		&cv.DeletedAt,
	}
}

func (cv *ConnectorDefinitionVersion) values() []any {
	return []any{
		cv.Id,
		cv.ConnectorId,
		cv.Version,
		cv.State,
		cv.EncryptedDefinition,
		cv.CreatedAt,
		cv.UpdatedAt,
		cv.EncryptedAt,
		cv.DeletedAt,
	}
}

func (s *service) selectConnectorDefinitionVersions() sq.SelectBuilder {
	return s.sq.
		Select(connectorWithDefinitionSelectCols()...).
		From(ConnectorDefinitionVersionsTable + " dv").
		Join(ConnectorsTable + " c ON c.id = dv.connector_id")
}

func (cv *ConnectorDefinitionVersion) Validate() error {
	result := &multierror.Error{}

	if cv.Id == apid.Nil {
		result = multierror.Append(result, errors.New("id is required"))
	}

	if err := cv.Id.ValidatePrefix(apid.PrefixConnectorDefinitionVersion); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector definition version id: %w", err))
	}

	if cv.ConnectorId == apid.Nil {
		result = multierror.Append(result, errors.New("connector id is required"))
	}

	if err := cv.ConnectorId.ValidatePrefix(apid.PrefixConnector); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector id: %w", err))
	}

	if cv.Version == 0 {
		result = multierror.Append(result, errors.New("version is required"))
	}

	if !IsValidConnectorDefinitionVersionState(cv.State) {
		result = multierror.Append(result, errors.New("invalid connector version state"))
	}

	if cv.EncryptedDefinition.IsZero() {
		result = multierror.Append(result, errors.New("encrypted definition is required"))
	}

	return result.ErrorOrNil()
}

func (s *service) GetConnectorDefinitionVersion(ctx context.Context, id apid.ID, version uint64) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorDefinitionVersions().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.version":      version,
			"dv.deleted_at":   nil,
			"c.deleted_at":    nil,
		}).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &result, nil
}

func (s *service) GetConnectorDefinitionVersions(
	ctx context.Context,
	requested []ConnectorDefinitionVersionId,
) (map[ConnectorDefinitionVersionId]*ConnectorWithDefinition, error) {
	if len(requested) == 0 {
		return nil, nil
	}

	ids := make(map[ConnectorDefinitionVersionId]struct{}, len(requested))
	for _, id := range requested {
		ids[id] = struct{}{}
	}

	versionConditions := util.Map(requested, func(id ConnectorDefinitionVersionId) sq.Sqlizer {
		return sq.Eq{"dv.connector_id": id.Id, "dv.version": id.Version}
	})

	rows, err := s.selectConnectorDefinitionVersions().
		Where(sq.And{
			sq.Eq{"c.deleted_at": nil, "dv.deleted_at": nil},
			sq.Or(versionConditions),
		}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []ConnectorWithDefinition
	for rows.Next() {
		var r ConnectorWithDefinition
		err := rows.Scan(r.fields()...)
		if err != nil {
			return nil, err
		}
		versions = append(versions, r)
	}

	versionMap := make(map[ConnectorDefinitionVersionId]*ConnectorWithDefinition, len(versions))
	for i := range versions {
		id := ConnectorDefinitionVersionId{
			Id:      versions[i].Id,
			Version: versions[i].Version,
		}
		if _, exists := ids[id]; exists {
			versionMap[id] = &versions[i]
		}
	}

	return versionMap, nil
}

func (s *service) UpsertConnectorDefinitionVersion(ctx context.Context, cv *ConnectorWithDefinition) error {
	if cv == nil {
		return errors.New("connector version is nil")
	}

	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectorId(cv.Id).
		Build()
	logger.Debug("upserting connector version")

	if validationErr := cv.Validate(); validationErr != nil {
		return validationErr
	}

	if cv.State != ConnectorDefinitionVersionStateDraft && cv.State != ConnectorDefinitionVersionStatePrimary {
		return errors.New("can only upsert connector version as draft or primary")
	}

	return s.transaction(func(tx *sql.Tx) error {
		sqb := s.sq.RunWith(tx)
		now := apctx.GetClock(ctx).Now()

		if err := s.ensureConnectorForDefinition(ctx, tx, cv); err != nil {
			return err
		}

		var existingState ConnectorDefinitionVersionState
		var existingCreatedAt time.Time
		err := sqb.
			Select("state").
			From(ConnectorDefinitionVersionsTable).
			Where(sq.Eq{"connector_id": cv.Id, "version": cv.Version, "deleted_at": nil}).
			Column("created_at").
			QueryRowContext(ctx).
			Scan(&existingState, &existingCreatedAt)
		existingRow := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if existingRow {
			if existingState != ConnectorDefinitionVersionStateDraft {
				logger.Error("cannot modify non-draft connector", "existing_state", existingState)
				return errors.New("cannot modify non-draft connector")
			}

			result, err := sqb.Update(ConnectorDefinitionVersionsTable).
				Set("state", cv.State).
				Set("encrypted_definition", cv.EncryptedDefinition).
				Set("updated_at", now).
				Set("encrypted_at", cv.EncryptedAt).
				Where(sq.Eq{"connector_id": cv.Id, "version": cv.Version, "deleted_at": nil}).
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
				return fmt.Errorf("expected to update 1 row for connector version, got %d", count)
			}
			cv.DefinitionCreatedAt = existingCreatedAt
			cv.DefinitionUpdatedAt = now
		} else {
			// No existing row at this version. Need to verify if there are existing rows, the new version is
			// existing version + 1
			maxVersion := uint64(0)
			err := sqb.
				Select("COALESCE(MAX(version), 0)").
				From(ConnectorDefinitionVersionsTable).
				Where(sq.Eq{"connector_id": cv.Id, "deleted_at": nil}).
				QueryRowContext(ctx).
				Scan(&maxVersion)

			if err != nil {
				return err
			}

			if maxVersion != 0 && maxVersion+1 != cv.Version {
				return errors.New("cannot insert connector version at non-sequential version")
			}

			definitionVersion := cv.definitionVersion()
			if definitionVersion.Id.IsNil() {
				definitionVersion.Id = apid.New(apid.PrefixConnectorDefinitionVersion)
			}
			definitionVersion.CreatedAt = now
			definitionVersion.UpdatedAt = now
			if err := definitionVersion.Validate(); err != nil {
				return err
			}

			_, err = sqb.Insert(ConnectorDefinitionVersionsTable).
				Columns(definitionVersion.cols()...).
				Values(definitionVersion.values()...).
				Exec()
			if err != nil {
				return err
			}
			cv.DefinitionVersionId = definitionVersion.Id
			cv.DefinitionCreatedAt = definitionVersion.CreatedAt
			cv.DefinitionUpdatedAt = definitionVersion.UpdatedAt
		}

		if cv.State == ConnectorDefinitionVersionStatePrimary {
			// New primary version, update any previous primary to active
			result, err := sqb.Update(ConnectorDefinitionVersionsTable).
				Set("state", ConnectorDefinitionVersionStateActive).
				Set("updated_at", now).
				Where(sq.And{
					sq.Eq{
						"connector_id": cv.Id,
						"state":        ConnectorDefinitionVersionStatePrimary,
						"deleted_at":   nil,
					},
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
			s.logger.Debug("updated connector versions from primary to active", "count", count)
		}

		return nil
	})
}

func (s *service) SetConnectorDefinitionVersionState(ctx context.Context, id apid.ID, version uint64, state ConnectorDefinitionVersionState) error {
	if id == apid.Nil {
		return errors.New("connector version id is required")
	}

	if !IsValidConnectorDefinitionVersionState(state) {
		return errors.New("invalid connector version state")
	}

	return s.transaction(func(tx *sql.Tx) error {
		sqb := s.sq.RunWith(tx)
		now := apctx.GetClock(ctx).Now()

		connectorResult, err := sqb.
			Update(ConnectorsTable).
			Set("updated_at", now).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to update connector timestamp: %w", err)
		}
		connectorsAffected, err := connectorResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to update connector timestamp: %w", err)
		}
		if connectorsAffected == 0 {
			return ErrNotFound
		}

		// Update the target version's state
		dbResult, err := sqb.
			Update(ConnectorDefinitionVersionsTable).
			Set("state", state).
			Set("updated_at", now).
			Where(sq.Eq{"connector_id": id, "version": version, "deleted_at": nil}).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to set connector version state: %w", err)
		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to set connector version state: %w", err)
		}

		if affected == 0 {
			return ErrNotFound
		}

		if affected > 1 {
			return fmt.Errorf("multiple connector versions had state updated: %w", ErrViolation)
		}

		if state == ConnectorDefinitionVersionStatePrimary {
			// Ensure only one primary: transition any other primary version to active
			_, err := sqb.Update(ConnectorDefinitionVersionsTable).
				Set("state", ConnectorDefinitionVersionStateActive).
				Set("updated_at", now).
				Where(sq.And{
					sq.Eq{"connector_id": id, "state": ConnectorDefinitionVersionStatePrimary, "deleted_at": nil},
					sq.NotEq{"version": version},
				}).
				Exec()
			if err != nil {
				return fmt.Errorf("failed to demote existing primary connector version: %w", err)
			}
		}

		if state == ConnectorDefinitionVersionStateDraft {
			// Ensure only one draft: transition any other draft version to archived
			_, err := sqb.Update(ConnectorDefinitionVersionsTable).
				Set("state", ConnectorDefinitionVersionStateArchived).
				Set("updated_at", now).
				Where(sq.And{
					sq.Eq{"connector_id": id, "state": ConnectorDefinitionVersionStateDraft, "deleted_at": nil},
					sq.NotEq{"version": version},
				}).
				Exec()
			if err != nil {
				return fmt.Errorf("failed to archive existing draft connector version: %w", err)
			}
		}

		return nil
	})
}

// DeleteConnector soft-deletes the logical connector, hiding all of its
// definition versions while retaining their history until the connector is
// hard-purged. Returns ErrNotFound if the logical connector is not live.
func (s *service) DeleteConnector(ctx context.Context, id apid.ID) error {
	if id == apid.Nil {
		return errors.New("connector id is required")
	}

	return s.transaction(func(tx *sql.Tx) error {
		sqb := s.sq.RunWith(tx)
		now := apctx.GetClock(ctx).Now()

		result, err := sqb.
			Update(ConnectorsTable).
			Set("updated_at", now).
			Set("deleted_at", now).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to soft delete connector: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to soft delete connector: %w", err)
		}
		if affected == 0 {
			return ErrNotFound
		}
		if affected > 1 {
			return fmt.Errorf("multiple connectors were soft deleted: %w", ErrViolation)
		}

		_, err = sqb.
			Update(ConnectorDefinitionVersionsTable).
			Set("updated_at", now).
			Set("deleted_at", now).
			Where(sq.Eq{"connector_id": id, "deleted_at": nil}).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to soft delete connector definition versions: %w", err)
		}

		return nil
	})
}

// GetConnectorDefinitionVersionForLabels finds the newest connector version matching the label selector.
func (s *service) GetConnectorDefinitionVersionForLabels(ctx context.Context, labelSelector string) (*ConnectorWithDefinition, error) {
	selector, err := ParseLabelSelector(labelSelector)
	if err != nil {
		return nil, err
	}

	var result ConnectorWithDefinition
	q := s.selectConnectorDefinitionVersions().
		Where(sq.Eq{"c.deleted_at": nil, "dv.deleted_at": nil})

	q = selector.ApplyToSqlBuilderWithProvider(q, "c.labels", s.cfg.GetProvider())

	err = q.OrderBy("c.created_at DESC", "dv.version DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// GetConnectorDefinitionVersionForLabelsAndVersion finds a connector version by labels + specific version.
func (s *service) GetConnectorDefinitionVersionForLabelsAndVersion(ctx context.Context, labelSelector string, version uint64) (*ConnectorWithDefinition, error) {
	selector, err := ParseLabelSelector(labelSelector)
	if err != nil {
		return nil, err
	}

	var result ConnectorWithDefinition
	q := s.selectConnectorDefinitionVersions().
		Where(sq.Eq{"dv.version": version, "dv.deleted_at": nil, "c.deleted_at": nil})

	q = selector.ApplyToSqlBuilderWithProvider(q, "c.labels", s.cfg.GetProvider())

	err = q.OrderBy("c.created_at DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *service) GetConnectorDefinitionVersionForState(ctx context.Context, id apid.ID, state ConnectorDefinitionVersionState) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorDefinitionVersions().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.state":        state,
			"dv.deleted_at":   nil,
			"c.deleted_at":    nil,
		}).
		OrderBy("dv.version DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &result, nil
}

func (s *service) NewestConnectorDefinitionVersionForId(ctx context.Context, id apid.ID) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorDefinitionVersions().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.deleted_at":   nil,
			"c.deleted_at":    nil,
		}).
		OrderBy("dv.version DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &result, nil
}

func (s *service) NewestPublishedConnectorDefinitionVersionForId(ctx context.Context, id apid.ID) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorDefinitionVersions().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.state":        []ConnectorDefinitionVersionState{ConnectorDefinitionVersionStatePrimary, ConnectorDefinitionVersionStateActive},
			"dv.deleted_at":   nil,
			"c.deleted_at":    nil,
		}).
		OrderBy("dv.version DESC").
		Limit(1).
		RunWith(s.db).
		QueryRow().
		Scan(result.fields()...)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &result, nil
}

type ConnectorDefinitionVersionOrderByField string

const (
	ConnectorDefinitionVersionOrderById        ConnectorDefinitionVersionOrderByField = "id"
	ConnectorDefinitionVersionOrderByVersion   ConnectorDefinitionVersionOrderByField = "version"
	ConnectorDefinitionVersionOrderByState     ConnectorDefinitionVersionOrderByField = "state"
	ConnectorDefinitionVersionOrderByCreatedAt ConnectorDefinitionVersionOrderByField = "created_at"
	ConnectorDefinitionVersionOrderByUpdatedAt ConnectorDefinitionVersionOrderByField = "updated_at"
)

func IsValidConnectorDefinitionVersionOrderByField[T string | ConnectorDefinitionVersionOrderByField](field T) bool {
	switch ConnectorDefinitionVersionOrderByField(field) {
	case ConnectorDefinitionVersionOrderById,
		ConnectorDefinitionVersionOrderByVersion,
		ConnectorDefinitionVersionOrderByState,
		ConnectorDefinitionVersionOrderByCreatedAt,
		ConnectorDefinitionVersionOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

type ListConnectorDefinitionVersionsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ConnectorWithDefinition]
	Enumerate(context.Context, pagination.EnumerateCallback[ConnectorWithDefinition]) error
}

type ListConnectorDefinitionVersionsBuilder interface {
	ListConnectorDefinitionVersionsExecutor
	Limit(int32) ListConnectorDefinitionVersionsBuilder
	ForId(apid.ID) ListConnectorDefinitionVersionsBuilder
	ForVersion(uint64) ListConnectorDefinitionVersionsBuilder
	ForState(ConnectorDefinitionVersionState) ListConnectorDefinitionVersionsBuilder
	ForStates([]ConnectorDefinitionVersionState) ListConnectorDefinitionVersionsBuilder
	ForNamespaceMatcher(string) ListConnectorDefinitionVersionsBuilder
	ForNamespaceMatchers([]string) ListConnectorDefinitionVersionsBuilder
	OrderBy(ConnectorDefinitionVersionOrderByField, pagination.OrderBy) ListConnectorDefinitionVersionsBuilder
	IncludeDeleted() ListConnectorDefinitionVersionsBuilder
	ForLabelSelector(selector string) ListConnectorDefinitionVersionsBuilder
}

type listConnectorDefinitionVersionsFilters struct {
	s                 *service                                `json:"-"`
	LimitVal          uint64                                  `json:"limit"`
	Offset            uint64                                  `json:"offset"`
	StatesVal         []ConnectorDefinitionVersionState       `json:"states,omitempty"`
	NamespaceMatchers []string                                `json:"namespace_matchers,omitempty"`
	IdsVal            []apid.ID                               `json:"ids,omitempty"`
	VersionsVal       []uint64                                `json:"versions,omitempty"`
	OrderByFieldVal   *ConnectorDefinitionVersionOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy                     `json:"order_by"`
	IncludeDeletedVal bool                                    `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                                 `json:"label_selector,omitempty"`
	Errors            *multierror.Error                       `json:"-"`
}

func (l *listConnectorDefinitionVersionsFilters) addError(e error) ListConnectorDefinitionVersionsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listConnectorDefinitionVersionsFilters) Limit(limit int32) ListConnectorDefinitionVersionsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listConnectorDefinitionVersionsFilters) ForState(state ConnectorDefinitionVersionState) ListConnectorDefinitionVersionsBuilder {
	l.StatesVal = []ConnectorDefinitionVersionState{state}
	return l
}

func (l *listConnectorDefinitionVersionsFilters) ForStates(states []ConnectorDefinitionVersionState) ListConnectorDefinitionVersionsBuilder {
	l.StatesVal = states
	return l
}

func (l *listConnectorDefinitionVersionsFilters) ForNamespaceMatcher(matcher string) ListConnectorDefinitionVersionsBuilder {
	if err := namespace.ValidateMatcher(matcher); err != nil {
		return l.addError(err)
	} else {
		l.NamespaceMatchers = []string{matcher}
	}

	return l
}

func (l *listConnectorDefinitionVersionsFilters) ForNamespaceMatchers(matchers []string) ListConnectorDefinitionVersionsBuilder {
	for _, matcher := range matchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listConnectorDefinitionVersionsFilters) ForId(id apid.ID) ListConnectorDefinitionVersionsBuilder {
	l.IdsVal = []apid.ID{id}
	return l
}

func (l *listConnectorDefinitionVersionsFilters) ForVersion(version uint64) ListConnectorDefinitionVersionsBuilder {
	l.VersionsVal = []uint64{version}
	return l
}

func (l *listConnectorDefinitionVersionsFilters) OrderBy(field ConnectorDefinitionVersionOrderByField, by pagination.OrderBy) ListConnectorDefinitionVersionsBuilder {
	if IsValidConnectorDefinitionVersionOrderByField(field) {
		l.OrderByFieldVal = &field
		l.OrderByVal = &by
	}
	return l
}

func (l *listConnectorDefinitionVersionsFilters) IncludeDeleted() ListConnectorDefinitionVersionsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectorDefinitionVersionsFilters) ForLabelSelector(selector string) ListConnectorDefinitionVersionsBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listConnectorDefinitionVersionsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectorDefinitionVersionsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listConnectorDefinitionVersionsFilters](ctx, s.cursorEncryptor, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listConnectorDefinitionVersionsFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.selectConnectorDefinitionVersions()

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			l.addError(err)
		} else {
			q = selector.ApplyToSqlBuilderWithProvider(q, "c.labels", l.s.cfg.GetProvider())
		}
	}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	if len(l.IdsVal) > 0 {
		q = q.Where(sq.Eq{"dv.connector_id": l.IdsVal})
	}

	if len(l.VersionsVal) > 0 {
		q = q.Where(sq.Eq{"dv.version": l.VersionsVal})
	}

	if len(l.StatesVal) > 0 {
		q = q.Where(sq.Eq{"dv.state": l.StatesVal})
	}

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "c.namespace", l.NamespaceMatchers)
	}

	if !l.IncludeDeletedVal {
		q = q.Where(sq.Eq{"c.deleted_at": nil, "dv.deleted_at": nil})
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(l.LimitVal + 1).Offset(l.Offset)

	if l.OrderByFieldVal != nil {
		orderCol := "dv." + string(*l.OrderByFieldVal)
		if *l.OrderByFieldVal == ConnectorDefinitionVersionOrderById {
			orderCol = "c.id"
		}
		q = q.OrderBy(fmt.Sprintf("%s %s", orderCol, l.OrderByVal.String()))
		if *l.OrderByFieldVal != ConnectorDefinitionVersionOrderById {
			q = q.OrderBy(fmt.Sprintf("c.id %s", l.OrderByVal.String()))
		}
		if *l.OrderByFieldVal != ConnectorDefinitionVersionOrderByVersion {
			q = q.OrderBy(fmt.Sprintf("dv.version %s", l.OrderByVal.String()))
		}
	}

	return q
}

func (l *listConnectorDefinitionVersionsFilters) fetchPage(ctx context.Context) pagination.PageResult[ConnectorWithDefinition] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[ConnectorWithDefinition]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[ConnectorWithDefinition]{Error: err}
	}
	defer rows.Close()

	var results []ConnectorWithDefinition
	for rows.Next() {
		var r ConnectorWithDefinition
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[ConnectorWithDefinition]{Error: err}
		}
		results = append(results, r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[ConnectorWithDefinition]{Error: err}
		}
	}

	return pagination.PageResult[ConnectorWithDefinition]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
		Cursor:  cursor,
	}
}

func (l *listConnectorDefinitionVersionsFilters) FetchPage(ctx context.Context) pagination.PageResult[ConnectorWithDefinition] {
	return l.fetchPage(ctx)
}

func (l *listConnectorDefinitionVersionsFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[ConnectorWithDefinition]) error {
	var err error
	keepGoing := pagination.Continue
	hasMore := true

	for err == nil && hasMore && bool(keepGoing) {
		result := l.FetchPage(ctx)
		hasMore = result.HasMore

		if result.Error != nil {
			return result.Error
		}
		keepGoing, err = callback(result)
	}

	return err
}

func (s *service) ListConnectorDefinitionVersionsBuilder() ListConnectorDefinitionVersionsBuilder {
	return &listConnectorDefinitionVersionsFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListConnectorDefinitionVersionsFromCursor(ctx context.Context, cursor string) (ListConnectorDefinitionVersionsExecutor, error) {
	b := &listConnectorDefinitionVersionsFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}
