package database

import (
	"context"
	"database/sql"
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
	// new connections. Existing connections of this connector will be upgraded to this version if possible, or
	// transitioned to a state where action is required to complete the upgrade.
	ConnectorVersionStatePrimary ConnectorVersionState = "primary"

	// ConnectorVersionStateActive means that a newer version of the connector has been published, but connections
	// still exist on this version that have not been upgraded.
	ConnectorVersionStateActive ConnectorVersionState = "active"

	// ConnectorVersionStateArchived means that this is an old version of the connect that does not have any active
	// connections running on the version.
	ConnectorVersionStateArchived ConnectorVersionState = "archived"
)

func IsValidConnectorVersionState[T string | ConnectorVersionState](state T) bool {
	switch ConnectorVersionState(state) {
	case ConnectorVersionStateDraft,
		ConnectorVersionStatePrimary,
		ConnectorVersionStateActive,
		ConnectorVersionStateArchived:
		return true
	default:
		return false
	}
}

const ConnectorVersionsTable = "connector_versions"

type ConnectorVersion struct {
	Id                  uuid.UUID
	Version             uint64
	Namespace           string
	State               ConnectorVersionState
	Hash                string
	EncryptedDefinition string
	Labels              Labels
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

func (cv *ConnectorVersion) cols() []string {
	return []string{
		"id",
		"version",
		"namespace",
		"state",
		"hash",
		"encrypted_definition",
		"labels",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (cv *ConnectorVersion) fields() []any {
	return []any{
		&cv.Id,
		&cv.Version,
		&cv.Namespace,
		&cv.State,
		&cv.Hash,
		&cv.EncryptedDefinition,
		&cv.Labels,
		&cv.CreatedAt,
		&cv.UpdatedAt,
		&cv.DeletedAt,
	}
}

func (cv *ConnectorVersion) values() []any {
	return []any{
		cv.Id,
		cv.Version,
		cv.Namespace,
		cv.State,
		cv.Hash,
		cv.EncryptedDefinition,
		cv.Labels,
		cv.CreatedAt,
		cv.UpdatedAt,
		cv.DeletedAt,
	}
}

func (cv *ConnectorVersion) GetId() uuid.UUID {
	return cv.Id
}

func (cv *ConnectorVersion) GetNamespace() string {
	return cv.Namespace
}

func (cv *ConnectorVersion) GetVersion() uint64 {
	return cv.Version
}

func (cv *ConnectorVersion) Validate() error {
	result := &multierror.Error{}

	if cv.Id == uuid.Nil {
		result = multierror.Append(result, errors.New("id is required"))
	}

	if cv.Version == 0 {
		result = multierror.Append(result, errors.New("version is required"))
	}

	if err := ValidateNamespacePath(cv.Namespace); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid connector namespace path"))
	}

	if !IsValidConnectorVersionState(cv.State) {
		result = multierror.Append(result, errors.New("invalid connector version state"))
	}

	if cv.Hash == "" {
		result = multierror.Append(result, errors.New("hash is required"))
	}

	if cv.EncryptedDefinition == "" {
		result = multierror.Append(result, errors.New("encrypted definition is required"))
	}

	if err := cv.Labels.Validate(); err != nil {
		result = multierror.Append(result, errors.Wrap(err, "invalid connector version labels"))
	}

	return result.ErrorOrNil()
}

func (s *service) GetConnectorVersion(ctx context.Context, id uuid.UUID, version uint64) (*ConnectorVersion, error) {
	var result ConnectorVersion
	err := s.sq.
		Select(result.cols()...).
		From(ConnectorVersionsTable).
		Where(sq.Eq{
			"id":         id,
			"version":    version,
			"deleted_at": nil,
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

func (s *service) GetConnectorVersions(
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

	versionConditions := util.Map(requested, func(id ConnectorVersionId) sq.Sqlizer {
		return sq.Eq{"id": id.Id, "version": id.Version}
	})

	rows, err := s.sq.
		Select(util.ToPtr(ConnectorVersion{}).cols()...).
		From(ConnectorVersionsTable).
		Where(sq.And{
			sq.Eq{"deleted_at": nil},
			sq.Or(versionConditions),
		}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []ConnectorVersion
	for rows.Next() {
		var r ConnectorVersion
		err := rows.Scan(r.fields()...)
		if err != nil {
			return nil, err
		}
		versions = append(versions, r)
	}

	versionMap := make(map[ConnectorVersionId]*ConnectorVersion, len(versions))
	for i := range versions {
		id := ConnectorVersionId{
			Id:      versions[i].Id,
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

func (s *service) UpsertConnectorVersion(ctx context.Context, cv *ConnectorVersion) error {
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

	if cv.State != ConnectorVersionStateDraft && cv.State != ConnectorVersionStatePrimary {
		return errors.New("can only upsert connector version as draft or primary")
	}

	return s.transaction(func(tx *sql.Tx) error {
		sqb := s.sq.RunWith(tx)

		existingNamespace, _, err := sqlh.ScanWithDefault(sqb.
			Select("namespace").
			From(ConnectorVersionsTable).
			Where(sq.Eq{"id": cv.Id, "version": cv.Version}).
			QueryRowContext(ctx),
			cv.Namespace)

		if err != nil {
			return err
		}

		if existingNamespace != cv.Namespace {
			return errors.New("cannot modify connector namespace")
		}

		exitingState, defaultUsed, err := sqlh.ScanWithDefault(sqb.
			Select("state").
			From(ConnectorVersionsTable).
			Where(sq.Eq{"id": cv.Id, "version": cv.Version}).
			QueryRowContext(ctx),
			ConnectorVersionStateDraft)

		existingRow := !defaultUsed
		if err != nil {
			return err
		}

		if existingRow {
			if exitingState != ConnectorVersionStateDraft {
				logger.Error("cannot modify non-draft connector", "existing_state", exitingState)
				return errors.New("cannot modify non-draft connector")
			}

			result, err := sqb.Update(ConnectorVersionsTable).
				Set("state", cv.State).
				Set("labels", cv.Labels).
				Set("encrypted_definition", cv.EncryptedDefinition).
				Set("updated_at", apctx.GetClock(ctx).Now()).
				Where(sq.Eq{"id": cv.Id, "version": cv.Version}).
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
				From(ConnectorVersionsTable).
				Where(sq.Eq{"id": cv.Id}).
				QueryRowContext(ctx).
				Scan(&maxVersion)

			if err != nil {
				return err
			}

			if maxVersion != 0 && maxVersion+1 != cv.Version {
				return errors.New("cannot insert connector version at non-sequential version")
			}

			cpy := *cv
			now := apctx.GetClock(ctx).Now()
			cpy.CreatedAt = now
			cpy.UpdatedAt = now

			_, err = sqb.Insert(ConnectorVersionsTable).
				Columns(cpy.cols()...).
				Values(cpy.values()...).
				Exec()
			if err != nil {
				return err
			}
		}

		if cv.State == ConnectorVersionStatePrimary {
			// New primary version, update any previous primary to active
			result, err := sqb.Update(ConnectorVersionsTable).
				Set("state", ConnectorVersionStateActive).
				Where(sq.And{
					sq.Eq{
						"id":    cv.Id,
						"state": ConnectorVersionStatePrimary,
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

// GetConnectorVersionForLabels finds the newest connector version matching the label selector.
func (s *service) GetConnectorVersionForLabels(ctx context.Context, labelSelector string) (*ConnectorVersion, error) {
	selector, err := ParseLabelSelector(labelSelector)
	if err != nil {
		return nil, err
	}

	var result ConnectorVersion
	q := s.sq.Select(result.cols()...).
		From(ConnectorVersionsTable).
		Where(sq.Eq{"deleted_at": nil})

	q = selector.ApplyToSqlBuilderWithProvider(q, "labels", s.cfg.GetProvider())

	err = q.OrderBy("created_at DESC", "version DESC").
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

// GetConnectorVersionForLabelsAndVersion finds a connector version by labels + specific version.
func (s *service) GetConnectorVersionForLabelsAndVersion(ctx context.Context, labelSelector string, version uint64) (*ConnectorVersion, error) {
	selector, err := ParseLabelSelector(labelSelector)
	if err != nil {
		return nil, err
	}

	var result ConnectorVersion
	q := s.sq.Select(result.cols()...).
		From(ConnectorVersionsTable).
		Where(sq.Eq{"version": version, "deleted_at": nil})

	q = selector.ApplyToSqlBuilderWithProvider(q, "labels", s.cfg.GetProvider())

	err = q.OrderBy("created_at DESC").
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

func (s *service) GetConnectorVersionForState(ctx context.Context, id uuid.UUID, state ConnectorVersionState) (*ConnectorVersion, error) {
	var result ConnectorVersion
	err := s.sq.
		Select(result.cols()...).
		From(ConnectorVersionsTable).
		Where(sq.Eq{
			"id":         id,
			"state":      state,
			"deleted_at": nil,
		}).
		OrderBy("version DESC").
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

func (s *service) NewestConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error) {
	var result ConnectorVersion
	err := s.sq.
		Select(result.cols()...).
		From(ConnectorVersionsTable).
		Where(sq.Eq{
			"id":         id,
			"deleted_at": nil,
		}).
		OrderBy("version DESC").
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

func (s *service) NewestPublishedConnectorVersionForId(ctx context.Context, id uuid.UUID) (*ConnectorVersion, error) {
	var result ConnectorVersion
	err := s.sq.
		Select(result.cols()...).
		From(ConnectorVersionsTable).
		Where(sq.Eq{
			"id":         id,
			"state":      []ConnectorVersionState{ConnectorVersionStatePrimary, ConnectorVersionStateActive},
			"deleted_at": nil,
		}).
		OrderBy("version DESC").
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

type ConnectorVersionOrderByField string

const (
	ConnectorVersionOrderById        ConnectorVersionOrderByField = "id"
	ConnectorVersionOrderByVersion   ConnectorVersionOrderByField = "version"
	ConnectorVersionOrderByState     ConnectorVersionOrderByField = "state"
	ConnectorVersionOrderByCreatedAt ConnectorVersionOrderByField = "created_at"
	ConnectorVersionOrderByUpdatedAt ConnectorVersionOrderByField = "updated_at"
)

func IsValidConnectorVersionOrderByField[T string | ConnectorVersionOrderByField](field T) bool {
	switch ConnectorVersionOrderByField(field) {
	case ConnectorVersionOrderById,
		ConnectorVersionOrderByVersion,
		ConnectorVersionOrderByState,
		ConnectorVersionOrderByCreatedAt,
		ConnectorVersionOrderByUpdatedAt:
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
	ForId(uuid.UUID) ListConnectorVersionsBuilder
	ForVersion(uint64) ListConnectorVersionsBuilder
	ForState(ConnectorVersionState) ListConnectorVersionsBuilder
	ForStates([]ConnectorVersionState) ListConnectorVersionsBuilder
	ForNamespaceMatcher(string) ListConnectorVersionsBuilder
	ForNamespaceMatchers([]string) ListConnectorVersionsBuilder
	OrderBy(ConnectorVersionOrderByField, pagination.OrderBy) ListConnectorVersionsBuilder
	IncludeDeleted() ListConnectorVersionsBuilder
	ForLabelSelector(selector string) ListConnectorVersionsBuilder
}

type listConnectorVersionsFilters struct {
	s                 *service                      `json:"-"`
	LimitVal          uint64                        `json:"limit"`
	Offset            uint64                        `json:"offset"`
	StatesVal         []ConnectorVersionState       `json:"states,omitempty"`
	NamespaceMatchers []string                      `json:"namespace_matchers,omitempty"`
	IdsVal            []uuid.UUID                   `json:"ids,omitempty"`
	VersionsVal       []uint64                      `json:"versions,omitempty"`
	OrderByFieldVal   *ConnectorVersionOrderByField `json:"order_by_field"`
	OrderByVal        *pagination.OrderBy           `json:"order_by"`
	IncludeDeletedVal bool                          `json:"include_deleted,omitempty"`
	LabelSelectorVal  *string                       `json:"label_selector,omitempty"`
	Errors            *multierror.Error             `json:"-"`
}

func (l *listConnectorVersionsFilters) addError(e error) ListConnectorVersionsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listConnectorVersionsFilters) Limit(limit int32) ListConnectorVersionsBuilder {
	l.LimitVal = uint64(limit)
	return l
}

func (l *listConnectorVersionsFilters) ForState(state ConnectorVersionState) ListConnectorVersionsBuilder {
	l.StatesVal = []ConnectorVersionState{state}
	return l
}

func (l *listConnectorVersionsFilters) ForStates(states []ConnectorVersionState) ListConnectorVersionsBuilder {
	l.StatesVal = states
	return l
}

func (l *listConnectorVersionsFilters) ForNamespaceMatcher(matcher string) ListConnectorVersionsBuilder {
	if err := ValidateNamespaceMatcher(matcher); err != nil {
		return l.addError(err)
	} else {
		l.NamespaceMatchers = []string{matcher}
	}

	return l
}

func (l *listConnectorVersionsFilters) ForNamespaceMatchers(matchers []string) ListConnectorVersionsBuilder {
	for _, matcher := range matchers {
		if err := ValidateNamespaceMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
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
	if IsValidConnectorVersionOrderByField(field) {
		l.OrderByFieldVal = &field
		l.OrderByVal = &by
	}
	return l
}

func (l *listConnectorVersionsFilters) IncludeDeleted() ListConnectorVersionsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectorVersionsFilters) ForLabelSelector(selector string) ListConnectorVersionsBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listConnectorVersionsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listConnectorVersionsFilters](ctx, s.secretKey, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listConnectorVersionsFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.sq.
		Select(util.ToPtr(ConnectorVersion{}).cols()...).
		From(ConnectorVersionsTable)

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			l.addError(err)
		} else {
			q = selector.ApplyToSqlBuilderWithProvider(q, "labels", l.s.cfg.GetProvider())
		}
	}

	if l.LimitVal <= 0 {
		l.LimitVal = 100
	}

	if len(l.IdsVal) > 0 {
		q = q.Where(sq.Eq{"id": l.IdsVal})
	}

	if len(l.VersionsVal) > 0 {
		q = q.Where(sq.Eq{"version": l.VersionsVal})
	}

	if len(l.StatesVal) > 0 {
		q = q.Where(sq.Eq{"states": l.StatesVal})
	}

	if len(l.NamespaceMatchers) > 0 {
		q = restrictToNamespaceMatchers(q, "namespace", l.NamespaceMatchers)
	}

	if !l.IncludeDeletedVal {
		q = q.Where(sq.Eq{"deleted_at": nil})
	}

	// Always limit to one more than limit to check if there are more records
	q = q.Limit(l.LimitVal + 1).Offset(l.Offset)

	if l.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", *l.OrderByFieldVal, l.OrderByVal.String()))
	}

	return q
}

func (l *listConnectorVersionsFilters) fetchPage(ctx context.Context) pagination.PageResult[ConnectorVersion] {
	var err error

	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[ConnectorVersion]{Error: err}
	}

	rows, err := l.applyRestrictions(ctx).
		RunWith(l.s.db).
		Query()
	if err != nil {
		return pagination.PageResult[ConnectorVersion]{Error: err}
	}
	defer rows.Close()

	var results []ConnectorVersion
	for rows.Next() {
		var r ConnectorVersion
		err := rows.Scan(r.fields()...)
		if err != nil {
			return pagination.PageResult[ConnectorVersion]{Error: err}
		}
		results = append(results, r)
	}

	l.Offset = l.Offset + uint64(len(results)) - 1 // we request one more than the page size we return

	cursor := ""
	hasMore := uint64(len(results)) > l.LimitVal
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.secretKey, l)
		if err != nil {
			return pagination.PageResult[ConnectorVersion]{Error: err}
		}
	}

	return pagination.PageResult[ConnectorVersion]{
		HasMore: hasMore,
		Results: results[:util.MinUint64(l.LimitVal, uint64(len(results)))],
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

func (s *service) ListConnectorVersionsBuilder() ListConnectorVersionsBuilder {
	return &listConnectorVersionsFilters{
		s:        s,
		LimitVal: 100,
	}
}

func (s *service) ListConnectorVersionsFromCursor(ctx context.Context, cursor string) (ListConnectorVersionsExecutor, error) {
	b := &listConnectorVersionsFilters{
		s:        s,
		LimitVal: 100,
	}

	return b.FromCursor(ctx, cursor)
}
