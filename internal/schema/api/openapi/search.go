package openapi

import "time"

// SearchResourcesResponseJson documents the bounded resource-search response.
type SearchResourcesResponseJson struct {
	// This anonymous shape mirrors schemaapi.SearchResourceSummaryJson while
	// avoiding swaggo's inability to resolve same-package nested named types.
	Items []struct {
		ResourceType  string            `json:"resourceType" example:"connection"`
		ResourceId    string            `json:"resourceId" example:"cxn_test550e8400abcde"`
		Name          string            `json:"name" example:"production-crm"`
		Namespace     string            `json:"namespace" example:"root.acme"`
		Labels        map[string]string `json:"labels"`
		MatchedLabels []struct {
			Key   string `json:"key" example:"name"`
			Value string `json:"value" example:"payments-production"`
		} `json:"matchedLabels"`
		UpdatedAt time.Time `json:"updatedAt"`
	} `json:"items"`
	TruncatedTypes  []string `json:"truncatedTypes"`
	IncompleteTypes []string `json:"incompleteTypes"`
}
