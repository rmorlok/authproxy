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
	ResourceType  SearchResourceType     `json:"resourceType" yaml:"resourceType" swaggertype:"string" example:"connection"`
	ResourceId    string                 `json:"resourceId" yaml:"resourceId" example:"cxn_test550e8400abcde"`
	Name          string                 `json:"name" yaml:"name" example:"production-crm"`
	Namespace     string                 `json:"namespace" yaml:"namespace" example:"root.acme"`
	Labels        map[string]string      `json:"labels" yaml:"labels"`
	MatchedLabels []SearchLabelMatchJson `json:"matchedLabels" yaml:"matchedLabels"`
	UpdatedAt     time.Time              `json:"updatedAt" yaml:"updatedAt"`
}

type SearchResourcesResponseJson struct {
	Items           []SearchResourceSummaryJson `json:"items" yaml:"items"`
	TruncatedTypes  []SearchResourceType        `json:"truncatedTypes" yaml:"truncatedTypes"`
	IncompleteTypes []SearchResourceType        `json:"incompleteTypes" yaml:"incompleteTypes"`
}
