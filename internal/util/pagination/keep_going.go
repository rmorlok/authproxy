package pagination

// KeepGoing is the bool returned by Enumerate-style callbacks to control iteration.
// Return true to continue to the next page; return false to stop early.
//
// Using this named type (instead of a bare bool) gives Enumerate callbacks across
// the codebase a consistent semantic: true means "keep going."
type KeepGoing = bool

const (
	// Continue tells an Enumerate-style iterator to fetch the next page.
	Continue KeepGoing = true

	// Stop tells an Enumerate-style iterator to halt before the next page.
	Stop KeepGoing = false
)
