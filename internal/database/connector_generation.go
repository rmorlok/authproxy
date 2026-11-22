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
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

type ConnectorGenerationState string

type ConnectorGenerationId struct {
	Id         apid.ID
	Generation uint64
}

// Value implements the driver.Valuer interface for ConnectorGenerationState
func (s ConnectorGenerationState) Value() (driver.Value, error) {
	return string(s), nil
}

// Scan implements the sql.Scanner interface for ConnectorGenerationState
func (s *ConnectorGenerationState) Scan(value interface{}) error {
	if value == nil {
		*s = ""
		return nil
	}

	strVal, ok := value.(string)
	if !ok {
		return fmt.Errorf("cannot convert %T to ConnectorGenerationState", value)
	}

	*s = ConnectorGenerationState(strVal)
	return nil
}

const (
	// ConnectorGenerationStateDraft means the connector definition is being worked on and new users should not connect to
	// this generation and existing users should not be upgraded to this generation
	ConnectorGenerationStateDraft ConnectorGenerationState = "draft"

	// ConnectorGenerationStatePrimary means that the generation has been published and this should be the generation used for
	// new connections. Existing connections of this connector will be upgraded to this generation if possible, or
	// transitioned to a state where action is required to complete the upgrade.
	ConnectorGenerationStatePrimary ConnectorGenerationState = "primary"

	// ConnectorGenerationStateActive means that a newer generation of the connector has been published, but connections
	// still exist on this generation that have not been upgraded.
	ConnectorGenerationStateActive ConnectorGenerationState = "active"

	// ConnectorGenerationStateArchived means that this is an old generation of the connect that does not have any active
	// connections running on the generation.
	ConnectorGenerationStateArchived ConnectorGenerationState = "archived"
)

func IsValidConnectorGenerationState[T string | ConnectorGenerationState](state T) bool {
	switch ConnectorGenerationState(state) {
	case ConnectorGenerationStateDraft,
		ConnectorGenerationStatePrimary,
		ConnectorGenerationStateActive,
		ConnectorGenerationStateArchived:
		return true
	default:
		return false
	}
}

func init() {
	RegisterEncryptedField(EncryptedFieldRegistration{
		Table:            ConnectorGenerationsTable,
		PrimaryKeyCols:   []string{"id"},
		EncryptedCols:    []string{"encrypted_definition"},
		JoinTable:        ConnectorsTable,
		JoinLocalCol:     "connector_id",
		JoinRemoteCol:    "id",
		JoinNamespaceCol: "namespace",
	})
}

const ConnectorGenerationsTable = "connector_generations"

// ConnectorGeneration is the database representation of a single row
// in connector_generations.
type ConnectorGeneration struct {
	Id                  apid.ID
	ConnectorId         apid.ID
	Generation          uint64
	State               ConnectorGenerationState
	EncryptedDefinition encfield.EncryptedField
	CreatedAt           time.Time
	UpdatedAt           time.Time
	EncryptedAt         *time.Time
	DeletedAt           *time.Time
}

func (cv *ConnectorGeneration) cols() []string {
	return []string{
		"id",
		"connector_id",
		"generation",
		"state",
		"encrypted_definition",
		"created_at",
		"updated_at",
		"encrypted_at",
		"deleted_at",
	}
}

func (cv *ConnectorGeneration) fields() []any {
	return []any{
		&cv.Id,
		&cv.ConnectorId,
		&cv.Generation,
		&cv.State,
		&cv.EncryptedDefinition,
		&cv.CreatedAt,
		&cv.UpdatedAt,
		&cv.EncryptedAt,
		&cv.DeletedAt,
	}
}

func (cv *ConnectorGeneration) values() []any {
	return []any{
		cv.Id,
		cv.ConnectorId,
		cv.Generation,
		cv.State,
		cv.EncryptedDefinition,
		cv.CreatedAt,
		cv.UpdatedAt,
		cv.EncryptedAt,
		cv.DeletedAt,
	}
}

func (s *service) selectConnectorGenerations() sq.SelectBuilder {
	return s.sq.
		Select(connectorWithDefinitionSelectCols()...).
		From(ConnectorGenerationsTable + " dv").
		Join(ConnectorsTable + " c ON c.id = dv.connector_id")
}

func (cv *ConnectorGeneration) Validate() error {
	result := &multierror.Error{}

	if cv.Id == apid.Nil {
		result = multierror.Append(result, errors.New("id is required"))
	}

	if err := cv.Id.ValidatePrefix(apid.PrefixConnectorGeneration); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector definition generation id: %w", err))
	}

	if cv.ConnectorId == apid.Nil {
		result = multierror.Append(result, errors.New("connector id is required"))
	}

	if err := cv.ConnectorId.ValidatePrefix(apid.PrefixConnector); err != nil {
		result = multierror.Append(result, fmt.Errorf("invalid connector id: %w", err))
	}

	if cv.Generation == 0 {
		result = multierror.Append(result, errors.New("generation is required"))
	}

	if !IsValidConnectorGenerationState(cv.State) {
		result = multierror.Append(result, errors.New("invalid connector generation state"))
	}

	if cv.EncryptedDefinition.IsZero() {
		result = multierror.Append(result, errors.New("encrypted definition is required"))
	}

	return result.ErrorOrNil()
}

func (s *service) GetConnectorGeneration(ctx context.Context, id apid.ID, generation uint64) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorGenerations().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.generation":   generation,
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

func (s *service) GetConnectorGenerations(
	ctx context.Context,
	requested []ConnectorGenerationId,
) (map[ConnectorGenerationId]*ConnectorWithDefinition, error) {
	if len(requested) == 0 {
		return nil, nil
	}

	ids := make(map[ConnectorGenerationId]struct{}, len(requested))
	for _, id := range requested {
		ids[id] = struct{}{}
	}

	generationConditions := util.Map(requested, func(id ConnectorGenerationId) sq.Sqlizer {
		return sq.Eq{"dv.connector_id": id.Id, "dv.generation": id.Generation}
	})

	rows, err := s.selectConnectorGenerations().
		Where(sq.And{
			sq.Eq{"c.deleted_at": nil, "dv.deleted_at": nil},
			sq.Or(generationConditions),
		}).
		RunWith(s.db).
		Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var generations []ConnectorWithDefinition
	for rows.Next() {
		var r ConnectorWithDefinition
		err := rows.Scan(r.fields()...)
		if err != nil {
			return nil, err
		}
		generations = append(generations, r)
	}

	generationMap := make(map[ConnectorGenerationId]*ConnectorWithDefinition, len(generations))
	for i := range generations {
		id := ConnectorGenerationId{
			Id:         generations[i].Id,
			Generation: generations[i].Generation,
		}
		if _, exists := ids[id]; exists {
			generationMap[id] = &generations[i]
		}
	}

	return generationMap, nil
}

func (s *service) UpsertConnectorGeneration(ctx context.Context, cv *ConnectorWithDefinition) error {
	if cv == nil {
		return errors.New("connector generation is nil")
	}

	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectorId(cv.Id).
		Build()
	logger.Debug("upserting connector generation")

	if validationErr := cv.Validate(); validationErr != nil {
		return validationErr
	}

	if cv.State != ConnectorGenerationStateDraft && cv.State != ConnectorGenerationStatePrimary {
		return errors.New("can only upsert connector generation as draft or primary")
	}

	return s.transaction(func(tx *sql.Tx) error {
		sqb := s.sq.RunWith(tx)
		now := apctx.GetClock(ctx).Now()

		if err := s.ensureConnectorForDefinition(ctx, tx, cv); err != nil {
			return err
		}

		var existingState ConnectorGenerationState
		var existingCreatedAt time.Time
		err := sqb.
			Select("state").
			From(ConnectorGenerationsTable).
			Where(sq.Eq{"connector_id": cv.Id, "generation": cv.Generation, "deleted_at": nil}).
			Column("created_at").
			QueryRowContext(ctx).
			Scan(&existingState, &existingCreatedAt)
		existingRow := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		if existingRow {
			if existingState != ConnectorGenerationStateDraft {
				logger.Error("cannot modify non-draft connector", "existing_state", existingState)
				return errors.New("cannot modify non-draft connector")
			}

			result, err := sqb.Update(ConnectorGenerationsTable).
				Set("state", cv.State).
				Set("encrypted_definition", cv.EncryptedDefinition).
				Set("updated_at", now).
				Set("encrypted_at", cv.EncryptedAt).
				Where(sq.Eq{"connector_id": cv.Id, "generation": cv.Generation, "deleted_at": nil}).
				Exec()
			if err != nil {
				return err
			}

			count, err := result.RowsAffected()
			if err != nil {
				return err
			}

			if count != 1 {
				logger.Error("expected to update 1 row for connector generation", "got", count)
				return fmt.Errorf("expected to update 1 row for connector generation, got %d", count)
			}
			cv.DefinitionCreatedAt = existingCreatedAt
			cv.DefinitionUpdatedAt = now
		} else {
			// No existing row at this generation. Need to verify if there are existing rows, the new generation is
			// existing generation + 1
			maxGeneration := uint64(0)
			err := sqb.
				Select("COALESCE(MAX(generation), 0)").
				From(ConnectorGenerationsTable).
				Where(sq.Eq{"connector_id": cv.Id, "deleted_at": nil}).
				QueryRowContext(ctx).
				Scan(&maxGeneration)

			if err != nil {
				return err
			}

			if maxGeneration != 0 && maxGeneration+1 != cv.Generation {
				return errors.New("cannot insert connector generation at non-sequential generation")
			}

			definitionGeneration := cv.definitionGeneration()
			if definitionGeneration.Id.IsNil() {
				definitionGeneration.Id = apid.New(apid.PrefixConnectorGeneration)
			}
			definitionGeneration.CreatedAt = now
			definitionGeneration.UpdatedAt = now
			if err := definitionGeneration.Validate(); err != nil {
				return err
			}

			_, err = sqb.Insert(ConnectorGenerationsTable).
				Columns(definitionGeneration.cols()...).
				Values(definitionGeneration.values()...).
				Exec()
			if err != nil {
				return err
			}
			cv.DefinitionGenerationId = definitionGeneration.Id
			cv.DefinitionCreatedAt = definitionGeneration.CreatedAt
			cv.DefinitionUpdatedAt = definitionGeneration.UpdatedAt
		}

		if cv.State == ConnectorGenerationStatePrimary {
			// New primary generation, update any previous primary to active
			result, err := sqb.Update(ConnectorGenerationsTable).
				Set("state", ConnectorGenerationStateActive).
				Set("updated_at", now).
				Where(sq.And{
					sq.Eq{
						"connector_id": cv.Id,
						"state":        ConnectorGenerationStatePrimary,
						"deleted_at":   nil,
					},
					sq.NotEq{"generation": cv.Generation},
				}).
				Exec()
			if err != nil {
				return err
			}

			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			s.logger.Debug("updated connector generations from primary to active", "count", count)
		}

		return nil
	})
}

func (s *service) SetConnectorGenerationState(ctx context.Context, id apid.ID, generation uint64, state ConnectorGenerationState) error {
	if id == apid.Nil {
		return errors.New("connector generation id is required")
	}

	if !IsValidConnectorGenerationState(state) {
		return errors.New("invalid connector generation state")
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

		// Update the target generation's state
		dbResult, err := sqb.
			Update(ConnectorGenerationsTable).
			Set("state", state).
			Set("updated_at", now).
			Where(sq.Eq{"connector_id": id, "generation": generation, "deleted_at": nil}).
			Exec()
		if err != nil {
			return fmt.Errorf("failed to set connector generation state: %w", err)
		}

		affected, err := dbResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to set connector generation state: %w", err)
		}

		if affected == 0 {
			return ErrNotFound
		}

		if affected > 1 {
			return fmt.Errorf("multiple connector generations had state updated: %w", ErrViolation)
		}

		if state == ConnectorGenerationStatePrimary {
			// Ensure only one primary: transition any other primary generation to active
			_, err := sqb.Update(ConnectorGenerationsTable).
				Set("state", ConnectorGenerationStateActive).
				Set("updated_at", now).
				Where(sq.And{
					sq.Eq{"connector_id": id, "state": ConnectorGenerationStatePrimary, "deleted_at": nil},
					sq.NotEq{"generation": generation},
				}).
				Exec()
			if err != nil {
				return fmt.Errorf("failed to demote existing primary connector generation: %w", err)
			}
		}

		if state == ConnectorGenerationStateDraft {
			// Ensure only one draft: transition any other draft generation to archived
			_, err := sqb.Update(ConnectorGenerationsTable).
				Set("state", ConnectorGenerationStateArchived).
				Set("updated_at", now).
				Where(sq.And{
					sq.Eq{"connector_id": id, "state": ConnectorGenerationStateDraft, "deleted_at": nil},
					sq.NotEq{"generation": generation},
				}).
				Exec()
			if err != nil {
				return fmt.Errorf("failed to archive existing draft connector generation: %w", err)
			}
		}

		return nil
	})
}

// DeleteConnector soft-deletes the logical connector, hiding all of its
// definition generations while retaining their history until the connector is
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
			Update(ConnectorGenerationsTable).
			Set("updated_at", now).
			Set("deleted_at", now).
			Where(sq.Eq{"connector_id": id, "deleted_at": nil}).
			ExecContext(ctx)
		if err != nil {
			return fmt.Errorf("failed to soft delete connector definition generations: %w", err)
		}

		return nil
	})
}

func (s *service) GetConnectorGenerationForState(ctx context.Context, id apid.ID, state ConnectorGenerationState) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorGenerations().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.state":        state,
			"dv.deleted_at":   nil,
			"c.deleted_at":    nil,
		}).
		OrderBy("dv.generation DESC").
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

func (s *service) NewestConnectorGenerationForId(ctx context.Context, id apid.ID) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorGenerations().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.deleted_at":   nil,
			"c.deleted_at":    nil,
		}).
		OrderBy("dv.generation DESC").
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

func (s *service) NewestPublishedConnectorGenerationForId(ctx context.Context, id apid.ID) (*ConnectorWithDefinition, error) {
	var result ConnectorWithDefinition
	err := s.selectConnectorGenerations().
		Where(sq.Eq{
			"dv.connector_id": id,
			"dv.state":        []ConnectorGenerationState{ConnectorGenerationStatePrimary, ConnectorGenerationStateActive},
			"dv.deleted_at":   nil,
			"c.deleted_at":    nil,
		}).
		OrderBy("dv.generation DESC").
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

type ConnectorGenerationOrderByField string

const (
	ConnectorGenerationOrderById         ConnectorGenerationOrderByField = "id"
	ConnectorGenerationOrderByGeneration ConnectorGenerationOrderByField = "generation"
	ConnectorGenerationOrderByState      ConnectorGenerationOrderByField = "state"
	ConnectorGenerationOrderByCreatedAt  ConnectorGenerationOrderByField = "created_at"
	ConnectorGenerationOrderByUpdatedAt  ConnectorGenerationOrderByField = "updated_at"
)

func IsValidConnectorGenerationOrderByField[T string | ConnectorGenerationOrderByField](field T) bool {
	switch ConnectorGenerationOrderByField(field) {
	case ConnectorGenerationOrderById,
		ConnectorGenerationOrderByGeneration,
		ConnectorGenerationOrderByState,
		ConnectorGenerationOrderByCreatedAt,
		ConnectorGenerationOrderByUpdatedAt:
		return true
	default:
		return false
	}
}

type ListConnectorGenerationsExecutor interface {
	FetchPage(context.Context) pagination.PageResult[ConnectorWithDefinition]
	Enumerate(context.Context, pagination.EnumerateCallback[ConnectorWithDefinition]) error
}

type ListConnectorGenerationsBuilder interface {
	ListConnectorGenerationsExecutor
	Limit(int32) ListConnectorGenerationsBuilder
	// ResumeAfter pages in key order (generation row id), continuing after a position
	// taken from PageResult.Position; an empty position starts at the beginning. It
	// cannot be combined with OrderBy.
	ResumeAfter(position string) ListConnectorGenerationsBuilder
	ForId(apid.ID) ListConnectorGenerationsBuilder
	ForGeneration(uint64) ListConnectorGenerationsBuilder
	ForState(ConnectorGenerationState) ListConnectorGenerationsBuilder
	ForStates([]ConnectorGenerationState) ListConnectorGenerationsBuilder
	ForNamespaceMatcher(string) ListConnectorGenerationsBuilder
	ForNamespaceMatchers([]string) ListConnectorGenerationsBuilder
	ForName(name scommon.ResourceName) ListConnectorGenerationsBuilder
	OrderBy(ConnectorGenerationOrderByField, pagination.OrderBy) ListConnectorGenerationsBuilder
	IncludeDeleted() ListConnectorGenerationsBuilder
	ForLabelSelector(selector string) ListConnectorGenerationsBuilder
}

// listConnectorGenerationsFilters is the state of a connector generation listing: its
// filters and its paging. Each row is one generation joined with its connector. It is
// serialized into encrypted cursors, so field JSON names are part of the cursor format.
type listConnectorGenerationsFilters struct {
	s                 *service                   `json:"-"`
	StatesVal         []ConnectorGenerationState `json:"states,omitempty"`
	NamespaceMatchers []string                   `json:"namespaceMatchers,omitempty"`
	// IdsVal restricts to generations of these connectors.
	IdsVal            []apid.ID             `json:"ids,omitempty"`
	GenerationsVal    []uint64              `json:"generations,omitempty"`
	NameVal           *scommon.ResourceName `json:"name,omitempty"`
	IncludeDeletedVal bool                  `json:"includeDeleted,omitempty"`
	LabelSelectorVal  *string               `json:"labelSelector,omitempty"`
	Errors            *multierror.Error     `json:"-"`

	// listPaging owns the page size, order and position. See listPaging.
	listPaging[ConnectorGenerationOrderByField]
}

func (l *listConnectorGenerationsFilters) addError(e error) ListConnectorGenerationsBuilder {
	l.Errors = multierror.Append(l.Errors, e)
	return l
}

func (l *listConnectorGenerationsFilters) Limit(limit int32) ListConnectorGenerationsBuilder {
	l.listPaging.setLimit(limit)
	return l
}

// ResumeAfter selects key order by generation row id, continuing after position.
func (l *listConnectorGenerationsFilters) ResumeAfter(position string) ListConnectorGenerationsBuilder {
	l.listPaging.resumeAfter(position)
	return l
}

func (l *listConnectorGenerationsFilters) ForState(state ConnectorGenerationState) ListConnectorGenerationsBuilder {
	l.StatesVal = []ConnectorGenerationState{state}
	return l
}

func (l *listConnectorGenerationsFilters) ForStates(states []ConnectorGenerationState) ListConnectorGenerationsBuilder {
	l.StatesVal = states
	return l
}

func (l *listConnectorGenerationsFilters) ForNamespaceMatcher(matcher string) ListConnectorGenerationsBuilder {
	if err := namespace.ValidateMatcher(matcher); err != nil {
		return l.addError(err)
	} else {
		l.NamespaceMatchers = []string{matcher}
	}

	return l
}

func (l *listConnectorGenerationsFilters) ForNamespaceMatchers(matchers []string) ListConnectorGenerationsBuilder {
	for _, matcher := range matchers {
		if err := namespace.ValidateMatcher(matcher); err != nil {
			return l.addError(err)
		}
	}
	l.NamespaceMatchers = matchers
	return l
}

func (l *listConnectorGenerationsFilters) ForId(id apid.ID) ListConnectorGenerationsBuilder {
	l.IdsVal = []apid.ID{id}
	return l
}

func (l *listConnectorGenerationsFilters) ForGeneration(generation uint64) ListConnectorGenerationsBuilder {
	l.GenerationsVal = []uint64{generation}
	return l
}

func (l *listConnectorGenerationsFilters) ForName(name scommon.ResourceName) ListConnectorGenerationsBuilder {
	if err := name.Validate(); err != nil {
		return l.addError(err)
	}
	l.NameVal = &name
	return l
}

func (l *listConnectorGenerationsFilters) OrderBy(field ConnectorGenerationOrderByField, by pagination.OrderBy) ListConnectorGenerationsBuilder {
	if IsValidConnectorGenerationOrderByField(field) {
		l.listPaging.setOrder(field, by)
	}
	return l
}

func (l *listConnectorGenerationsFilters) IncludeDeleted() ListConnectorGenerationsBuilder {
	l.IncludeDeletedVal = true
	return l
}

func (l *listConnectorGenerationsFilters) ForLabelSelector(selector string) ListConnectorGenerationsBuilder {
	l.LabelSelectorVal = &selector
	return l
}

func (l *listConnectorGenerationsFilters) FromCursor(ctx context.Context, cursor string) (ListConnectorGenerationsExecutor, error) {
	s := l.s
	parsed, err := pagination.ParseCursor[listConnectorGenerationsFilters](ctx, s.cursorEncryptor, cursor)

	if err != nil {
		return nil, err
	}

	*l = *parsed
	l.s = s

	return l, nil
}

func (l *listConnectorGenerationsFilters) applyRestrictions(ctx context.Context) sq.SelectBuilder {
	q := l.s.selectConnectorGenerations()

	if l.LabelSelectorVal != nil {
		selector, err := ParseLabelSelector(*l.LabelSelectorVal)
		if err != nil {
			l.addError(err)
		} else {
			q = selector.ApplyToSqlBuilderWithProvider(q, "c.labels", l.s.cfg.GetProvider())
		}
	}

	if len(l.IdsVal) > 0 {
		q = q.Where(sq.Eq{"dv.connector_id": l.IdsVal})
	}

	if len(l.GenerationsVal) > 0 {
		q = q.Where(sq.Eq{"dv.generation": l.GenerationsVal})
	}

	if l.NameVal != nil {
		q = q.Where(sq.Eq{"c.name": *l.NameVal})
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

	// The key is the generation row's own id, which is unique and never changes. With
	// no order chosen, each connector's generations are listed oldest first, grouped
	// by connector id.
	q = l.listPaging.applyOrdered(q, "dv.id", connectorGenerationOrderClauses, "c.id ASC", "dv.generation ASC")

	return q
}

// connectorGenerationOrderClauses maps a caller-chosen order field to ORDER BY
// clauses over the generation listing's query, where dv is the generation and c its
// connector. The id field means the connector id. Connector id and generation number
// break ties, in the same direction, so the order is total.
func connectorGenerationOrderClauses(field ConnectorGenerationOrderByField, direction string) []string {
	orderCol := "dv." + string(field)
	if field == ConnectorGenerationOrderById {
		orderCol = "c.id"
	}
	clauses := []string{fmt.Sprintf("%s %s", orderCol, direction)}
	if field != ConnectorGenerationOrderById {
		clauses = append(clauses, fmt.Sprintf("c.id %s", direction))
	}
	if field != ConnectorGenerationOrderByGeneration {
		clauses = append(clauses, fmt.Sprintf("dv.generation %s", direction))
	}
	return clauses
}

func (l *listConnectorGenerationsFilters) fetchPage(ctx context.Context) pagination.PageResult[ConnectorWithDefinition] {
	var err error

	if err = l.listPaging.validate(); err != nil {
		return pagination.PageResult[ConnectorWithDefinition]{Error: err}
	}

	// applyRestrictions can record label selector errors, so check them after it.
	q := l.applyRestrictions(ctx)
	if err = l.Errors.ErrorOrNil(); err != nil {
		return pagination.PageResult[ConnectorWithDefinition]{Error: err}
	}

	rows, err := q.
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
	if err = rows.Err(); err != nil {
		return pagination.PageResult[ConnectorWithDefinition]{Error: err}
	}

	page, hasMore, position := nextPage(&l.listPaging, results, func(r ConnectorWithDefinition) string {
		return string(r.DefinitionGenerationId)
	})

	cursor := ""
	if hasMore {
		cursor, err = pagination.MakeCursor(ctx, l.s.cursorEncryptor, l)
		if err != nil {
			return pagination.PageResult[ConnectorWithDefinition]{Error: err}
		}
	}

	return pagination.PageResult[ConnectorWithDefinition]{
		HasMore:  hasMore,
		Results:  page,
		Cursor:   cursor,
		Position: position,
	}
}

func (l *listConnectorGenerationsFilters) FetchPage(ctx context.Context) pagination.PageResult[ConnectorWithDefinition] {
	return l.fetchPage(ctx)
}

// Enumerate walks every page, calling callback for each. Unless the caller chose an
// order or continued from an offset cursor, it walks in generation row id order, so
// writes made during the walk, such as archiving the generations it visits, do not
// make it skip or repeat generations.
func (l *listConnectorGenerationsFilters) Enumerate(ctx context.Context, callback pagination.EnumerateCallback[ConnectorWithDefinition]) error {
	l.listPaging.startEnumerate()

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

func (s *service) ListConnectorGenerationsBuilder() ListConnectorGenerationsBuilder {
	return &listConnectorGenerationsFilters{
		s: s,
	}
}

func (s *service) ListConnectorGenerationsFromCursor(ctx context.Context, cursor string) (ListConnectorGenerationsExecutor, error) {
	b := &listConnectorGenerationsFilters{
		s: s,
	}

	return b.FromCursor(ctx, cursor)
}
