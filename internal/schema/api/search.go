package api

import "time"

type SearchResourceType string

const (
	SearchResourceTypeActor      SearchResourceType = "actor"
	SearchResourceTypeConnection SearchResourceType = "connection"
	SearchResourceTypeConnector  SearchResourceType = "connector"
	SearchResourceTypeNamespace  SearchResourceType = "namespace"
	SearchResourceTypeKey        SearchResourceType = "key"
	SearchResourceTypeRateLimit  SearchResourceType = "rate_limit"
)

type SearchLabelMatchJson struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}

type SearchResourceSummaryJson struct {
	ResourceType  SearchResourceType     `json:"resource_type" yaml:"resource_type" swaggertype:"string" example:"connection"`
	ResourceId    string                 `json:"resource_id" yaml:"resource_id" example:"cxn_test550e8400abcde"`
	Namespace     string                 `json:"namespace" yaml:"namespace" example:"root.acme"`
	Labels        map[string]string      `json:"labels" yaml:"labels"`
	MatchedLabels []SearchLabelMatchJson `json:"matched_labels" yaml:"matched_labels"`
	UpdatedAt     time.Time              `json:"updated_at" yaml:"updated_at"`
}

type SearchResourcesResponseJson struct {
	Items           []SearchResourceSummaryJson `json:"items" yaml:"items"`
	TruncatedTypes  []SearchResourceType        `json:"truncated_types" yaml:"truncated_types"`
	IncompleteTypes []SearchResourceType        `json:"incomplete_types" yaml:"incomplete_types"`
}
