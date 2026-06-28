package namespace

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/rmorlok/authproxy/internal/util"
)

var validPathRegex = regexp.MustCompile(`^root(?:\.[a-zA-Z0-9_]+[a-zA-Z0-9_\-]*)*$`)

// Root represents the base namespace for all hierarchical paths in the system. Other namespaces
// follow from this path using PathSeparator, e.g. root.child.grandchild.
const Root = "root"

// PathSeparator is the character used to separate namespace parts in a path.
const PathSeparator = "."

// Wildcard is the recursive namespace matcher token.
const Wildcard = "**"

// WildcardSuffix is appended to a namespace to indicate all child namespaces are included.
// For example, "root.**" matches "root", "root.foo", "root.foo.bar", etc. This constant should be used
// if a namespace matches a recursive wildcard. Using wildcard directly leads to matching with foo.bar**
const WildcardSuffix = PathSeparator + Wildcard

// SkipPermissionChecks is a sentinel value used to indicate that the namespace is not currently
// known at the time of permission checking. During permission checking, at the API layer namespace won't generally be
// known, so permission checking starts with resource and verb. This value is used to indicate that
// namespace checking for permissions should be ignored.
const SkipPermissionChecks = "<SKIP_NAMESPACE_PERMISSION_CHECK>"

// NoMatchSentinel is a sentinel value used to indicate that the set of allowed namespaces is an empty set.
// This is a value used to indicate that the intersection of requested and allowed namespaces is empty.
const NoMatchSentinel = "<NO_MATCH>"

// ValidatePath checks if the path is valid. It returns an error if it is not with a descriptive message.
func ValidatePath(path string) error {
	if path == "" {
		return errors.New("path is required")
	}

	if path == SkipPermissionChecks {
		// This wouldn't be valid anyway, but to just be explicit to guard against future changes.
		return errors.New("disallowed sentinel value")
	}

	if path != Root && !strings.HasPrefix(path, Root+PathSeparator) {
		return errors.New("path must be a child of root")
	}

	if !validPathRegex.MatchString(path) {
		return errors.New("path can only contain lowercase letters a-z")
	}

	return nil
}

// ValidateMatcher checks if the matcher is valid. A matcher is valid if it is a valid path or is is a
// valid path appended by ".**". This method assumes the matcher must be present.
func ValidateMatcher(matcher string) error {
	if matcher == "" {
		return errors.New("namespace matcher is required")
	}

	if matcher != Root && !strings.HasPrefix(matcher, Root+PathSeparator) {
		return errors.New("matcher must start with root")
	}

	if strings.HasSuffix(matcher, ".**") {
		return ValidatePath(matcher[:len(matcher)-3])
	} else {
		return ValidatePath(matcher)
	}
}

// DepthOfPath returns the number of path segments in the given path. This is a measure of how deep from
// root this path is. So root has depth 0, root.foo has depth 1, root.foo.bar has depth 2, etc.
func DepthOfPath(path string) uint64 {
	return uint64(util.MaxInt(len(util.Filter(strings.Split(path, PathSeparator), func(s string) bool { return s != "" }))-1, 0))
}

// SplitPathToPrefixes returns all the prefix paths for a given path, including the given path.
//
// So if the path is "root.foo.bar", it will return ["root", "root.foo", "root.foo.bar"]. The output will be
// ordered in increasing path length.
func SplitPathToPrefixes(path string) []string {
	if path == "" {
		return []string{}
	}

	parts := strings.Split(path, PathSeparator)
	result := make([]string, len(parts))

	for i := 0; i < len(parts); i++ {
		result[i] = strings.Join(parts[0:i+1], PathSeparator)
	}

	return result
}

// SplitPathsToPrefixes returns all the prefix paths for a given set of paths. The prefixes will be the set
// of paths that are common to all the given paths, once for each prefix. The output includes the paths themselves.
// Output is returning in ascending order of path depth with a deterministic sub-ordering.
//
// So if paths is ["root.foo.bar", "roo.baz"], it will return ["root", "root.baz", "root.foo", "root.foo.bar"].
func SplitPathsToPrefixes(paths []string) []string {
	if len(paths) == 0 {
		return []string{}
	}

	prefixSet := make(map[string]struct{})

	for _, path := range paths {
		for _, prefix := range SplitPathToPrefixes(path) {
			prefixSet[prefix] = struct{}{}
		}
	}

	result := util.GetKeys(prefixSet)

	slices.SortFunc(result, func(i, j string) int {
		iDepth := DepthOfPath(i)
		jDepth := DepthOfPath(j)

		if iDepth != jDepth {
			return int(iDepth - jDepth)
		}

		if i < j {
			return -1
		}

		if i > j {
			return 1
		}

		return 0
	})

	return result
}

// PathFromRoot returns a namespace path from the given parts, prefixed with "root.".
func PathFromRoot(parts ...string) string {
	allPaths := append([]string{Root}, parts...)
	return strings.Join(allPaths, PathSeparator)
}

// ParentPath returns the parent namespace path for a valid namespace path.
// The parent of root is root.
func ParentPath(path string) string {
	i := strings.LastIndex(path, PathSeparator)
	if i < 0 {
		return Root
	}

	return path[:i]
}

// IsChild returns true if the child path is a child of the parent path.
func IsChild(parentPath, childPath string) bool {
	return strings.HasPrefix(childPath, parentPath+PathSeparator)
}

// IsSameOrChild returns true if the parent path is the same as the child path,
// or if the child path is a child of the parent path.
func IsSameOrChild(parentPath, childPath string) bool {
	return parentPath == childPath || IsChild(parentPath, childPath)
}

// ConstrainMatcher returns a constrained namespace matcher to the most constrained intersection of the
// two namespace matchers, including consideration for wildcard matching. The return value is the constrained matcher
// and a boolean indicating if the operation was successful.
func ConstrainMatcher(ns1, ns2 string) (constrained string, ok bool) {
	if ns1 == "" || ns2 == "" {
		return "", false
	}

	if ns1 == ns2 {
		return ns1, true
	}

	shorter := ns1
	longer := ns2

	if len(ns1) > len(ns2) {
		shorter = ns2
		longer = ns1
	}

	if shorter == longer[:len(longer)-len(WildcardSuffix)] {
		// This covers case (root.child, root.child.**) -> root.child
		return shorter, true
	}

	shorterHasWildcard := strings.HasSuffix(shorter, WildcardSuffix)

	if !shorterHasWildcard {
		// The shorter namespace doesn't have a wildcard, so it can't be a parent of the longer namespace
		return "", false
	}

	if strings.HasPrefix(longer, shorter[:len(shorter)-len(Wildcard)]) {
		// The shorter namespace is a parent of the longer namespace
		// and it has wildcards, so return the longer namespace
		return longer, true
	}

	return "", false
}

// Matches determines if a given namespace matches a matcher, considering support for wildcard matching.
func Matches(matcher, namespace string) bool {
	if matcher == "" || namespace == "" {
		return false
	}

	// Check for wildcard namespace (e.g., "root.**")
	if strings.HasSuffix(matcher, WildcardSuffix) {
		baseNamespace := matcher[:len(matcher)-len(WildcardSuffix)]
		// Match the base namespace itself or any child namespace
		return namespace == baseNamespace || IsChild(baseNamespace, namespace)
	}

	// Exact match
	return matcher == namespace
}
