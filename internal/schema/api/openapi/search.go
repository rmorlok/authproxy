package openapi

import "time"

// SearchResourcesResponseJson documents the bounded resource-search response.
type SearchResourcesResponseJson struct {
	// This anonymous shape mirrors schemaapi.SearchResourceSummaryJson while
	// avoiding swaggo's inability to resolve same-package nested named types.
	Items []struct {
		ResourceType  string            `json:"resource_type" example:"connection"`
		ResourceId    string            `json:"resource_id" example:"cxn_test550e8400abcde"`
		Namespace     string            `json:"namespace" example:"root.acme"`
		Labels        map[string]string `json:"labels"`
		MatchedLabels []struct {
			Key   string `json:"key" example:"name"`
			Value string `json:"value" example:"payments-production"`
		} `json:"matched_labels"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"items"`
	TruncatedTypes  []string `json:"truncated_types"`
	IncompleteTypes []string `json:"incomplete_types"`
}
