package openapi

import (
	"time"

	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
)

type SearchResultListMetaJson struct {
	TruncatedKinds  []string `json:"truncatedKinds"`
	IncompleteKinds []string `json:"incompleteKinds"`
}

// SearchResourcesResponseJson documents the heterogeneous resource-search
// projection. Each result carries one typed ObjectReference rather than a
// second flat identity envelope.
type SearchResourcesResponseJson struct {
	APIVersion string                   `json:"apiVersion" binding:"required" enums:"authproxy.net/v1alpha1" example:"authproxy.net/v1alpha1"`
	Kind       string                   `json:"kind" binding:"required" enums:"SearchResultList" example:"SearchResultList"`
	Metadata   SearchResultListMetaJson `json:"metadata" binding:"required"`
	Items      []struct {
		ResourceRef   meta.ObjectReference `json:"resourceRef" binding:"required"`
		Labels        map[string]string    `json:"labels"`
		MatchedLabels []struct {
			Key   string `json:"key" example:"team"`
			Value string `json:"value" example:"payments"`
		} `json:"matchedLabels"`
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"items" binding:"required"`
}
