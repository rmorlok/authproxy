package pagination

// PageResult is the result of a paged query
type PageResult[T any] struct {
	// Results is the list of results for this page
	Results []T

	// HasMore indicates whether there are more results to fetch
	HasMore bool

	// Cursor is the cursor to use to fetch the next page of results
	Cursor string

	// Position is the resume key of the last row returned, for passing to a builder's
	// ResumeAfter. Unlike Cursor it is plain text, holds no filters, and stays set on
	// the last page. It is set only by key-ordered database listings, and core
	// wrappers do not carry it.
	Position string

	// Error is set if there was an error fetching the results
	Error error

	// Total is the total number of results available. This is an optional value depending on the system providing
	// the paginated results.
	Total *int64
}
