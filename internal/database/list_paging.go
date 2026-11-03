package database

import (
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/rmorlok/authproxy/internal/util/pagination"
)

// defaultListPageSize is the page size of a listing whose caller did not set a limit.
const defaultListPageSize = 100

// listPaging owns how a listing moves through its rows: the page size, and the order
// and position of the walk. F is the listing's order-by field type.
//
// A listing walks in one of two modes:
//   - Offset: rows in the caller's order, or the listing's default order, skipping
//     Offset rows. Removing rows during the walk shifts the ones after them under the
//     offset, so an offset walk that deletes or repairs what it visits skips rows.
//   - Key: rows in order of a unique, immutable key, after the key in After. The
//     position stays exact while rows are updated or leave the listing.
//
// Key mode is chosen by resumeAfter, and by startEnumerate when the caller chose no
// order. It cannot be combined with a caller-chosen order; validate reports that.
//
// It is embedded in list filters, so its fields join the encrypted cursor payload
// with their JSON names unchanged, and a cursor keeps the mode it was taken in.
type listPaging[F ~string] struct {
	// LimitVal is the page size. Zero selects defaultListPageSize when the query is
	// built.
	LimitVal uint64 `json:"limit"`
	// Offset is the number of rows already returned in offset mode. Unused in key mode.
	Offset uint64 `json:"offset"`
	// OrderByFieldVal is the caller-chosen order column, or nil for the default order.
	OrderByFieldVal *F `json:"orderByField"`
	// OrderByVal is the direction of OrderByFieldVal.
	OrderByVal *pagination.OrderBy `json:"orderBy"`
	// Keyset selects key mode.
	Keyset bool `json:"keyset,omitempty"`
	// After is the key of the last row returned in key mode, or empty before the
	// first row.
	After string `json:"afterKey,omitempty"`
}

// setLimit sets the page size. Zero or less selects the default.
func (p *listPaging[F]) setLimit(limit int32) {
	if limit < 0 {
		limit = 0
	}
	p.LimitVal = uint64(limit)
}

// setOrder sets the caller-chosen order. Callers validate field against the
// listing's allowed fields first.
func (p *listPaging[F]) setOrder(field F, by pagination.OrderBy) {
	p.OrderByFieldVal = &field
	p.OrderByVal = &by
}

// resumeAfter selects key mode, continuing after position. An empty position starts
// at the beginning.
func (p *listPaging[F]) resumeAfter(position string) {
	p.Keyset = true
	p.After = position
}

// startEnumerate selects key mode for an Enumerate call unless the caller chose an
// order, or the listing is already partway through its pages by offset, as a listing
// resumed from an offset cursor is. Key mode keeps writes made during the walk from
// moving its position.
func (p *listPaging[F]) startEnumerate() {
	if p.Offset == 0 && p.OrderByFieldVal == nil {
		p.Keyset = true
	}
}

// validate rejects key mode combined with a caller-chosen order, because a key
// position is meaningless in any other order. The error wraps ErrInvalidListQuery.
func (p *listPaging[F]) validate() error {
	if p.Keyset && p.OrderByFieldVal != nil {
		return fmt.Errorf("%w: %w", ErrInvalidListQuery, errors.New("a resume position cannot be combined with an explicit order"))
	}
	return nil
}

// apply adds paging to q. It fetches one row more than the page size so nextPage can
// tell whether more rows follow, defaulting the page size first if unset.
//
// In key mode it orders by keyColumn and restricts to rows after the position;
// keyColumn must be unique and immutable, and unqualified or qualified to match the
// query. In offset mode it orders by the caller's field, or by defaultOrder when the
// caller chose none, and skips Offset rows. defaultOrder may be empty, though a
// listing without a stable order can repeat or skip rows across pages.
func (p *listPaging[F]) apply(q sq.SelectBuilder, keyColumn string, defaultOrder ...string) sq.SelectBuilder {
	if p.LimitVal == 0 {
		p.LimitVal = defaultListPageSize
	}
	q = q.Limit(p.LimitVal + 1)

	if p.Keyset {
		if p.After != "" {
			q = q.Where(sq.Gt{keyColumn: p.After})
		}
		return q.OrderBy(keyColumn + " ASC")
	}

	if p.OrderByFieldVal != nil {
		q = q.OrderBy(fmt.Sprintf("%s %s", string(*p.OrderByFieldVal), p.OrderByVal.String()))
	} else if len(defaultOrder) > 0 {
		q = q.OrderBy(defaultOrder...)
	}
	return q.Offset(p.Offset)
}

// nextPage takes the rows fetched by a query built with apply and moves p past them.
// It returns the rows to hand back, whether more rows follow, and in key mode the
// position to resume from, read from the last row with key. An empty page leaves the
// position unchanged, so a listing that ran out of rows still reports where it
// stopped. Offset mode returns an empty position.
//
// It is a function rather than a method because Go methods cannot take the row type
// as a type parameter.
func nextPage[F ~string, T any](p *listPaging[F], rows []T, key func(T) string) (page []T, hasMore bool, position string) {
	hasMore = uint64(len(rows)) > p.LimitVal
	page = rows
	if hasMore {
		page = rows[:p.LimitVal]
	}

	if !p.Keyset {
		p.Offset += uint64(len(page))
		return page, hasMore, ""
	}

	if len(page) > 0 {
		p.After = key(page[len(page)-1])
	}
	return page, hasMore, p.After
}
