package api

import (
	"time"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

const SearchResultListKind meta.Kind = "SearchResultList"

type SearchLabelMatchJson struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}

// SearchResourceSummaryJson is a heterogeneous resource projection. Its
// ResourceRef owns identity; the remaining fields contain search-only display
// and match information rather than a partial resource envelope.
type SearchResourceSummaryJson struct {
	ResourceRef   meta.ObjectReference   `json:"resourceRef" yaml:"resourceRef"`
	Labels        map[string]string      `json:"labels" yaml:"labels"`
	MatchedLabels []SearchLabelMatchJson `json:"matchedLabels" yaml:"matchedLabels"`
	UpdatedAt     time.Time              `json:"updatedAt" yaml:"updatedAt"`
}

// SearchResultListMetaJson reports partial-result state for the heterogeneous
// search projection. It is not resource pagination metadata.
type SearchResultListMetaJson struct {
	TruncatedKinds  []meta.Kind `json:"truncatedKinds" yaml:"truncatedKinds"`
	IncompleteKinds []meta.Kind `json:"incompleteKinds" yaml:"incompleteKinds"`
}

// SearchResourcesResponseJson is a typed list projection rather than a list
// of partial canonical resources.
type SearchResourcesResponseJson struct {
	meta.TypeMeta `json:",inline" yaml:",inline"`
	Metadata      SearchResultListMetaJson    `json:"metadata" yaml:"metadata"`
	Items         []SearchResourceSummaryJson `json:"items" yaml:"items"`
}

func NewSearchResourcesResponseJson(
	items []SearchResourceSummaryJson,
	truncatedKinds, incompleteKinds []meta.Kind,
) SearchResourcesResponseJson {
	if items == nil {
		items = make([]SearchResourceSummaryJson, 0)
	}
	if truncatedKinds == nil {
		truncatedKinds = make([]meta.Kind, 0)
	}
	if incompleteKinds == nil {
		incompleteKinds = make([]meta.Kind, 0)
	}
	return SearchResourcesResponseJson{
		TypeMeta: meta.NewTypeMeta(SearchResultListKind),
		Metadata: SearchResultListMetaJson{
			TruncatedKinds:  truncatedKinds,
			IncompleteKinds: incompleteKinds,
		},
		Items: items,
	}
}
