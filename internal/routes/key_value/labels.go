// Package key_value provides a generic adapter for exposing the standard
// labels and annotations endpoints on resources that store key/value
// metadata in a uniform way:
//
//	GET    /<resource>/<id>/<segment>            list all
//	GET    /<resource>/<id>/<segment>/<key>      get one
//	PUT    /<resource>/<id>/<segment>/<key>      set one
//	DELETE /<resource>/<id>/<segment>/<key>      delete one
//
// Each resource provides an Adapter wired to its own ID type, fetcher,
// and persistence functions; the handler implementations live here once.
package key_value

import (
	"github.com/rmorlok/authproxy/internal/database"
)

// Resource is the minimal contract a fetched resource must satisfy so the
// adapter can read its labels and annotations. Resources are also passed
// through to the auth validator, which uses runtime type assertions to
// extract namespace and id, so the concrete type must additionally
// satisfy the validator's expectations (typically GetNamespace() string
// and GetId() apid.ID, or a custom id extractor configured on the auth
// builder).
type Resource interface {
	GetLabels() map[string]string
	GetAnnotations() map[string]string
}

// Kind captures the differences between labels and annotations that the
// generic adapter needs to vary on: URL segment naming, validation
// rules, and which accessor returns the relevant map.
type Kind struct {
	// PathSegment is the URL segment ("labels" or "annotations").
	PathSegment string
	// ParamName is the gin path-parameter name for the key
	// ("label" or "annotation").
	ParamName string
	// Singular is the noun used in error messages ("label" or "annotation").
	Singular string
	// ValidateKey validates a single key.
	ValidateKey func(string) error
	// ValidateValue validates a single value.
	ValidateValue func(string) error
	// Get returns the relevant map from a Resource.
	Get func(Resource) map[string]string
}

// Label is the Kind for labels.
var Label = Kind{
	PathSegment:   "labels",
	ParamName:     "label",
	Singular:      "label",
	ValidateKey:   database.ValidateLabelKey,
	ValidateValue: database.ValidateLabelValue,
	Get:           func(r Resource) map[string]string { return r.GetLabels() },
}

// Annotation is the Kind for annotations.
var Annotation = Kind{
	PathSegment:   "annotations",
	ParamName:     "annotation",
	Singular:      "annotation",
	ValidateKey:   database.ValidateAnnotationKey,
	ValidateValue: database.ValidateAnnotationValue,
	Get:           func(r Resource) map[string]string { return r.GetAnnotations() },
}

// KeyValueJson is a single key/value pair returned by the get-one and
// put endpoints for both labels and annotations.
type KeyValueJson struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PutKeyValueRequestJson is the request body for PUT
// /<resource>/<id>/labels/<key> and the annotation equivalent.
type PutKeyValueRequestJson struct {
	Value string `json:"value"`
}
