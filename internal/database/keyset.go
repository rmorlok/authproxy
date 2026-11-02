package database

import (
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
)

// keysetWalk is the position of a listing that pages by a unique, immutable key
// rather than by offset. A key position stays exact while rows are updated or leave
// the listing, which an offset does not: removing rows shifts the ones after them
// under the offset, so an offset walk that deletes or repairs what it visits skips
// rows.
//
// It is embedded in list filters, so its fields join the encrypted cursor payload
// and a cursor taken from a key-ordered listing stays key-ordered.
type keysetWalk struct {
	// Enabled selects key order. It is set by ResumeAfter, and by Enumerate on a
	// listing with no caller-chosen order.
	Enabled bool `json:"keyset,omitempty"`
	// After is the key of the last row returned, or empty before the first row.
	After string `json:"afterKey,omitempty"`
}

// resumeAfter selects key order, continuing after position. An empty position
// starts at the beginning.
func (k *keysetWalk) resumeAfter(position string) {
	k.Enabled = true
	k.After = position
}

// enableForEnumerate selects key order for an Enumerate call unless the caller chose
// an order, or the listing is already partway through its pages by offset, as a
// listing resumed from an offset cursor is. offset is the listing's current offset
// and ordered reports whether the caller chose an order.
func (k *keysetWalk) enableForEnumerate(offset uint64, ordered bool) {
	if offset == 0 && !ordered {
		k.Enabled = true
	}
}

// validate rejects a key-ordered listing that also has a caller-chosen order,
// because a key position is meaningless in any other order. ordered reports whether
// the caller chose an order. The error wraps ErrInvalidListQuery.
func (k *keysetWalk) validate(ordered bool) error {
	if k.Enabled && ordered {
		return fmt.Errorf("%w: %w", ErrInvalidListQuery, errors.New("a resume position cannot be combined with an explicit order"))
	}
	return nil
}

// apply restricts q to the rows after the position and orders it by column. column
// must be unique and immutable, and unqualified or qualified to match the query.
func (k *keysetWalk) apply(q sq.SelectBuilder, column string) sq.SelectBuilder {
	if k.After != "" {
		q = q.Where(sq.Gt{column: k.After})
	}
	return q.OrderBy(column + " ASC")
}

// advanceKeyset moves walk past the last row of page, using key to read that row's
// key, and returns the position to resume from. An empty page leaves the position
// unchanged, so a listing that ran out of rows still reports where it stopped.
func advanceKeyset[T any](walk *keysetWalk, page []T, key func(T) string) string {
	if len(page) > 0 {
		walk.After = key(page[len(page)-1])
	}
	return walk.After
}
