package auth

import ns "github.com/rmorlok/authproxy/internal/schema/resources/namespace"

const RootNamespace = ns.RootNamespace
const NamespacePathSeparator = ns.NamespacePathSeparator
const NamespaceWildcard = ns.NamespaceWildcard
const NamespaceWildcardSuffix = ns.NamespaceWildcardSuffix
const NamespaceSkipNamespacePermissionChecks = ns.NamespaceSkipNamespacePermissionChecks
const NamespaceNoMatchSentinel = ns.NamespaceNoMatchSentinel

func ValidateNamespacePath(path string) error {
	return ns.ValidateNamespacePath(path)
}

func ValidateNamespaceMatcher(matcher string) error {
	return ns.ValidateNamespaceMatcher(matcher)
}

func DepthOfNamespacePath(path string) uint64 {
	return ns.DepthOfNamespacePath(path)
}

func SplitNamespacePathToPrefixes(path string) []string {
	return ns.SplitNamespacePathToPrefixes(path)
}

func SplitNamespacePathsToPrefixes(paths []string) []string {
	return ns.SplitNamespacePathsToPrefixes(paths)
}

func NamespacePathFromRoot(parts ...string) string {
	return ns.NamespacePathFromRoot(parts...)
}

func NamespaceIsChild(parentPath, childPath string) bool {
	return ns.NamespaceIsChild(parentPath, childPath)
}

func NamespaceIsSameOrChild(parentPath, childPath string) bool {
	return ns.NamespaceIsSameOrChild(parentPath, childPath)
}

func NamespaceMatcherConstrained(ns1, ns2 string) (constrained string, ok bool) {
	return ns.NamespaceMatcherConstrained(ns1, ns2)
}

func NamespaceMatches(matcher, namespace string) bool {
	return ns.NamespaceMatches(matcher, namespace)
}
