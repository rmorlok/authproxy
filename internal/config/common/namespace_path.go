package common

import (
	"errors"
	"regexp"
	"strings"
)

var validPathRegex = regexp.MustCompile(`^root(?:/[a-zA-Z0-9_]+[a-zA-Z0-9_\-]*)*$`)

// ValidateNamespacePath checks if the path is valid. It returns an error if it is not with a descriptive message.
func ValidateNamespacePath(path string) error {
	if path == "" {
		return errors.New("path is required")
	}

	if path != "root" && !strings.HasPrefix(path, "root/") {
		return errors.New("path must be a child of root")
	}

	if !validPathRegex.MatchString(path) {
		return errors.New("path can only contain lowercase letters a-z")
	}

	return nil
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
