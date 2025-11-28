package common

import (
	"errors"
	"regexp"
	"strings"

	"github.com/rmorlok/authproxy/internal/util"
)

var validPathRegex = regexp.MustCompile(`^root(?:/[a-zA-Z0-9_]+[a-zA-Z0-9_\-]*)*$`)

const RootNamespace = "root"

// ValidateNamespacePath checks if the path is valid. It returns an error if it is not with a descriptive message.
func ValidateNamespacePath(path string) error {
	if path == "" {
		return errors.New("path is required")
	}

	if path != RootNamespace && !strings.HasPrefix(path, RootNamespace+"/") {
		return errors.New("path must be a child of root")
	}

	if !validPathRegex.MatchString(path) {
		return errors.New("path can only contain lowercase letters a-z")
	}

	return nil
}

// DepthOfNamespacePath returns the number of path segments in the given path. This is a measure of how deep from
// root this path is. So root has depth 0, root/foo has depth 1, root/foo/bar has depth 2, etc.
func DepthOfNamespacePath(path string) uint64 {
	return uint64(util.MaxInt(len(util.Filter(strings.Split(path, "/"), func(s string) bool { return s != "" }))-1, 0))
}

// SplitNamespacePathToPrefixes returns all the prefix paths for a given path, including the given path.
//
// So if the path is "root/foo/bar", it will return ["root", "root/foo", "root/foo/bar"]. The output will be
// ordered in increasing path length.
func SplitNamespacePathToPrefixes(path string) []string {
	if path == "" {
		return []string{}
	}

	parts := strings.Split(path, "/")
	result := make([]string, len(parts))

	for i := 0; i < len(parts); i++ {
		result[i] = strings.Join(parts[0:i+1], "/")
	}

	return result
}

// NamespacePathFromRoot returns a namespace path from the given parts, prefixed with "root/".
func NamespacePathFromRoot(parts ...string) string {
	allPaths := append([]string{RootNamespace}, parts...)
	return strings.Join(allPaths, "/")
}

// NamespaceIsChild returns true if the child path is a child of the parent path.
func NamespaceIsChild(parentPath, childPath string) bool {
	return strings.HasPrefix(childPath, parentPath+"/")
}

// NamespaceIsSameOrChild returns true if the parent path is the same as the child path,
// or if the child path is a child of the parent path.
func NamespaceIsSameOrChild(parentPath, childPath string) bool {
	return parentPath == childPath || NamespaceIsChild(parentPath, childPath)
}
