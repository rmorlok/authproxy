package meta

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	APIGroup                      = "authproxy.net"
	APIVersionName                = "v1alpha1"
	APIVersionV1Alpha1 APIVersion = APIGroup + "/" + APIVersionName
)

var (
	apiGroupPattern = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?\.)*[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	versionPattern  = regexp.MustCompile(`^v[0-9]+(?:(?:alpha|beta)[0-9]+)?$`)
	kindPattern     = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
)

// APIVersion identifies a group and schema version independently of the HTTP
// route version.
type APIVersion string

// ParseAPIVersion validates Kubernetes-style group/version syntax. Whether a
// syntactically valid version is registered is the manifest scheme's concern.
func ParseAPIVersion(value string) (APIVersion, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || len(parts[0]) > 253 || !apiGroupPattern.MatchString(parts[0]) || !versionPattern.MatchString(parts[1]) {
		return "", fmt.Errorf("must use group/version syntax, for example %q", APIVersionV1Alpha1)
	}
	return APIVersion(value), nil
}

func (v APIVersion) Validate() error {
	_, err := ParseAPIVersion(string(v))
	return err
}

func (v APIVersion) Group() string {
	if parsed, err := ParseAPIVersion(string(v)); err == nil {
		return strings.SplitN(string(parsed), "/", 2)[0]
	}
	return ""
}

func (v APIVersion) Version() string {
	if parsed, err := ParseAPIVersion(string(v)); err == nil {
		return strings.SplitN(string(parsed), "/", 2)[1]
	}
	return ""
}

// Kind is a singular PascalCase resource, list, projection, or action kind.
type Kind string

func (k Kind) Validate() error {
	if !kindPattern.MatchString(string(k)) {
		return fmt.Errorf("must be a non-empty PascalCase kind")
	}
	return nil
}

func NewTypeMeta(kind Kind) TypeMeta {
	return TypeMeta{APIVersion: APIVersionV1Alpha1, Kind: kind}
}
