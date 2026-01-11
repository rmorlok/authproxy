package auth

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/rmorlok/authproxy/internal/util"
)

var validPathRegex = regexp.MustCompile(`^root(?:\.[a-zA-Z0-9_]+[a-zA-Z0-9_\-]*)*$`)

// RootNamespace represents the base namespace for all hierarchical paths in the system. Other namespaces
// follow from this path using NamespacePathSeparator, e.g. root.child.grandchild
const RootNamespace = "root"

// NamespacePathSeparator is the character used to separate namespace parts in a path.
const NamespacePathSeparator = "."

// NamespaceSkipNamespacePermissionChecks is a sentinel value used to indicate that the namespace is not currently
// known at the time of permission checking. During permission checking, at the API layer namespace won't generally be
// known, so permission checking starts with resource and  verb. This essential value is used to indicate that
// namespace checking for permissions should be ignored.
const NamespaceSkipNamespacePermissionChecks = "<SKIP_NAMESPACE_PERMISSION_CHECK>"

// ValidateNamespacePath checks if the path is valid. It returns an error if it is not with a descriptive message.
func ValidateNamespacePath(path string) error {
	if path == "" {
		return errors.New("path is required")
	}

	if path == NamespaceSkipNamespacePermissionChecks {
		// This wouldn't be valid anyway, but to just be explicit to guard against future changes.
		return errors.New("disallowed sentinel value")
	}

	if path != RootNamespace && !strings.HasPrefix(path, RootNamespace+NamespacePathSeparator) {
		return errors.New("path must be a child of root")
	}

	if !validPathRegex.MatchString(path) {
		return errors.New("path can only contain lowercase letters a-z")
	}

	return nil
}

// ValidateNamespaceMatcher checks if the matcher is valid. A matcher is valid if it is a valid path or is is a
// valid path appended by ".**". This method assumes the matcher must be present.
func ValidateNamespaceMatcher(matcher string) error {
	if matcher == "" {
		return errors.New("namespace matcher is required")
	}

	if matcher != RootNamespace && !strings.HasPrefix(matcher, RootNamespace+NamespacePathSeparator) {
		return errors.New("matcher must start with root")
	}

	if strings.HasSuffix(matcher, ".**") {
		return ValidateNamespacePath(matcher[:len(matcher)-3])
	} else {
		return ValidateNamespacePath(matcher)
	}
}

// DepthOfNamespacePath returns the number of path segments in the given path. This is a measure of how deep from
// root this path is. So root has depth 0, root/foo has depth 1, root/foo/bar has depth 2, etc.
func DepthOfNamespacePath(path string) uint64 {
	return uint64(util.MaxInt(len(util.Filter(strings.Split(path, NamespacePathSeparator), func(s string) bool { return s != "" }))-1, 0))
}

// SplitNamespacePathToPrefixes returns all the prefix paths for a given path, including the given path.
//
// So if the path is "root/foo/bar", it will return ["root", "root/foo", "root/foo/bar"]. The output will be
// ordered in increasing path length.
func SplitNamespacePathToPrefixes(path string) []string {
	if path == "" {
		return []string{}
	}

	parts := strings.Split(path, NamespacePathSeparator)
	result := make([]string, len(parts))

	for i := 0; i < len(parts); i++ {
		result[i] = strings.Join(parts[0:i+1], NamespacePathSeparator)
	}

	return result
}

// SplitNamespacePathsToPrefixes returns all the prefix paths for a given set of paths. The prefixes will be the set
// of paths that are common to all the given paths, once for each prefix. The output includes the paths themselves.
// Output is returning in ascending order of path depth with a deterministic sub-ordering.
//
// So if paths is ["root.foo.bar", "roo.baz"], it will return ["root", "root.baz", "root.foo", "root.foo.bar"].
func SplitNamespacePathsToPrefixes(paths []string) []string {
	if len(paths) == 0 {
		return []string{}
	}

	prefixSet := make(map[string]struct{})

	for _, path := range paths {
		for _, prefix := range SplitNamespacePathToPrefixes(path) {
			prefixSet[prefix] = struct{}{}
		}
	}

	result := util.GetKeys(prefixSet)

	slices.SortFunc(result, func(i, j string) int {
		iDepth := DepthOfNamespacePath(i)
		jDepth := DepthOfNamespacePath(j)

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

// NamespacePathFromRoot returns a namespace path from the given parts, prefixed with "root/".
func NamespacePathFromRoot(parts ...string) string {
	allPaths := append([]string{RootNamespace}, parts...)
	return strings.Join(allPaths, NamespacePathSeparator)
}

// NamespaceIsChild returns true if the child path is a child of the parent path.
func NamespaceIsChild(parentPath, childPath string) bool {
	return strings.HasPrefix(childPath, parentPath+NamespacePathSeparator)
}

// NamespaceIsSameOrChild returns true if the parent path is the same as the child path,
// or if the child path is a child of the parent path.
func NamespaceIsSameOrChild(parentPath, childPath string) bool {
	return parentPath == childPath || NamespaceIsChild(parentPath, childPath)
}
