package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/apid"
	scommon "github.com/rmorlok/authproxy/internal/schema/common"
	smeta "github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

const ConnectorsTable = "connectors"

// Connector is the database representation of a logical connector. Definition
// versions are stored separately in connector_definition_versions.
type Connector struct {
	Id          apid.ID
	Namespace   string
	Name        scommon.ResourceName
	Labels      Labels
	Annotations Annotations
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

func (c *Connector) cols() []string {
	return []string{
		"id",
		"namespace",
		"name",
		"labels",
		"annotations",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func (c *Connector) fields() []any {
	return []any{
		&c.Id,
		&c.Namespace,
		&c.Name,
		&c.Labels,
		&c.Annotations,
		&c.CreatedAt,
		&c.UpdatedAt,
		&c.DeletedAt,
	}
}

func (c *Connector) values() []any {
	return []any{
		c.Id,
		c.Namespace,
		c.Name,
		c.Labels,
		c.Annotations,
		c.CreatedAt,
		c.UpdatedAt,
		c.DeletedAt,
	}
}

func (c *Connector) Validate() error {
	if c.Id.IsNil() {
		return errors.New("connector id is required")
	}
	if err := c.Id.ValidatePrefix(apid.PrefixConnector); err != nil {
		return fmt.Errorf("invalid connector id: %w", err)
	}
	if err := namespace.ValidatePath(c.Namespace); err != nil {
		return fmt.Errorf("invalid connector namespace: %w", err)
	}
	if err := c.Name.Validate(); err != nil {
		return fmt.Errorf("invalid connector name: %w", err)
	}
	if err := c.Labels.Validate(); err != nil {
		return fmt.Errorf("invalid connector labels: %w", err)
	}
	if err := c.Annotations.Validate(); err != nil {
		return fmt.Errorf("invalid connector annotations: %w", err)
	}
	return nil
}

func (s *service) ensureConnectorForDefinition(
	ctx context.Context,
	tx *sql.Tx,
	cv *ConnectorWithDefinition,
) error {
	sqb := s.sq.RunWith(tx)
	var existing Connector
	err := sqb.
		Select(existing.cols()...).
		From(ConnectorsTable).
		Where(sq.Eq{"id": cv.Id, "deleted_at": nil}).
		QueryRowContext(ctx).
		Scan(existing.fields()...)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if err == nil {
		if existing.Namespace != cv.Namespace {
			return errors.New("cannot modify connector namespace through a connector version")
		}
		if cv.Name != "" && cv.Name != existing.Name {
			return errors.New("cannot modify connector name through a connector version")
		}

		labels := existing.Labels
		if cv.Labels != nil {
			labels = MergeUpsertLabels(cv.Labels, existing.Labels)
			labels = InjectSelfImplicitLabels(existing.Id, existing.Name, existing.Namespace, labels)
		}
		annotations := existing.Annotations
		if cv.Annotations != nil {
			annotations = cv.Annotations
		}
		now := apctx.GetClock(ctx).Now()
		_, err = sqb.
			Update(ConnectorsTable).
			Set("labels", labels).
			Set("annotations", annotations).
			Set("updated_at", now).
			Where(sq.Eq{"id": existing.Id, "deleted_at": nil}).
			ExecContext(ctx)
		if err != nil {
			return err
		}

		cv.Name = existing.Name
		cv.Labels = labels
		cv.Annotations = annotations
		cv.CreatedAt = existing.CreatedAt
		cv.UpdatedAt = now
		cv.DeletedAt = existing.DeletedAt
		return nil
	}

	name := cv.Name
	if name == "" {
		name = scommon.ResourceName(cv.Id.String())
	}
	nsLabels, err := s.fetchLabelsForCarryForward(ctx, tx, NamespacesTable, sq.Eq{
		"path":       cv.Namespace,
		"deleted_at": nil,
	})
	if err != nil {
		return err
	}
	labels := ApplyParentCarryForward(
		cv.Labels,
		ParentCarryForward{Rt: NamespaceLabelToken, Labels: nsLabels},
	)
	labels = InjectSelfImplicitLabels(cv.Id, name, cv.Namespace, labels)

	now := apctx.GetClock(ctx).Now()
	connector := Connector{
		Id:          cv.Id,
		Namespace:   cv.Namespace,
		Name:        name,
		Labels:      labels,
		Annotations: cv.Annotations,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := connector.Validate(); err != nil {
		return err
	}

	_, err = sqb.
		Insert(ConnectorsTable).
		Columns(connector.cols()...).
		Values(connector.values()...).
		ExecContext(ctx)
	if err != nil {
		return wrapDatabaseMutationError("failed to create connector", err)
	}
	cv.Name = connector.Name
	cv.Labels = connector.Labels
	cv.Annotations = connector.Annotations
	cv.CreatedAt = connector.CreatedAt
	cv.UpdatedAt = connector.UpdatedAt
	return nil
}

// UpdateConnectorName renames one live logical connector without changing any
// immutable connector-version definition rows.
func (s *service) UpdateConnectorName(ctx context.Context, id apid.ID, name scommon.ResourceName) error {
	if id.IsNil() {
		return errors.New("connector id is required")
	}
	if err := id.ValidatePrefix(apid.PrefixConnector); err != nil {
		return fmt.Errorf("invalid connector id: %w", err)
	}
	if err := name.Validate(); err != nil {
		return fmt.Errorf("invalid connector name: %w", err)
	}

	return s.updateResourceNameAndSelfLabels(ctx, ConnectorsTable, id, name)
}

// UpdateConnectorLabels replaces all user-owned labels on a logical connector
// while preserving its system-managed labels.
func (s *service) UpdateConnectorLabels(ctx context.Context, id apid.ID, labels map[string]string) (*Connector, error) {
	if id.IsNil() {
		return nil, errors.New("connector id is required")
	}
	if labels != nil {
		if err := smeta.ValidateUserLabels(labels); err != nil {
			return nil, fmt.Errorf("invalid labels: %w", err)
		}
	}

	var result *Connector
	err := s.transaction(func(tx *sql.Tx) error {
		var connector Connector
		err := s.sq.
			Select(connector.cols()...).
			From(ConnectorsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRowContext(ctx).
			Scan(connector.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		merged, now, err := s.replaceUserLabelsInTableTx(ctx, tx, ConnectorsTable, sq.Eq{"id": id, "deleted_at": nil}, Labels(labels))
		if err != nil {
			return err
		}
		connector.Labels = merged
		connector.UpdatedAt = now
		result = &connector
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateConnectorAnnotations replaces all annotations on a logical connector.
func (s *service) UpdateConnectorAnnotations(ctx context.Context, id apid.ID, annotations map[string]string) (*Connector, error) {
	if id.IsNil() {
		return nil, errors.New("connector id is required")
	}
	if annotations != nil {
		if err := smeta.ValidateAnnotations(annotations); err != nil {
			return nil, fmt.Errorf("invalid annotations: %w", err)
		}
	}

	var result *Connector
	err := s.transaction(func(tx *sql.Tx) error {
		var connector Connector
		err := s.sq.
			Select(connector.cols()...).
			From(ConnectorsTable).
			Where(sq.Eq{"id": id, "deleted_at": nil}).
			RunWith(tx).
			QueryRowContext(ctx).
			Scan(connector.fields()...)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		now, err := s.updateAnnotationsInTableTx(ctx, tx, ConnectorsTable, sq.Eq{"id": id, "deleted_at": nil}, Annotations(annotations))
		if err != nil {
			return err
		}
		connector.Annotations = Annotations(annotations)
		connector.UpdatedAt = now
		result = &connector
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
