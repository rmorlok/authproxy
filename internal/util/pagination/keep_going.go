package pagination

// KeepGoing is the bool returned by Enumerate-style callbacks to control iteration.
// Return Continue to fetch the next page; return Stop to halt early.
//
// Defined as a named type (not an alias) so callbacks must return the exported
// constants — bare true/false won't compile, which keeps intent explicit.
type KeepGoing bool

const (
	// Continue tells an Enumerate-style iterator to fetch the next page.
	Continue KeepGoing = true

	// Stop tells an Enumerate-style iterator to halt before the next page.
	Stop KeepGoing = false
)
