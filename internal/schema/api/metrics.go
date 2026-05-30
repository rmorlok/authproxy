package api

import "time"

// MetricsRangeJson describes the time range and bucket size for a metrics query.
//
//	@Description	Time range and bucket size for a metrics query
type MetricsRangeJson struct {
	Start time.Time `json:"start" yaml:"start" example:"2026-05-25T12:00:00Z"`
	End   time.Time `json:"end" yaml:"end" example:"2026-05-25T13:00:00Z"`
	Step  string    `json:"step" yaml:"step" example:"15m"`
}

// MetricsQueryRefJson describes one metric series request within a query.
//
//	@Description	One metric series request
type MetricsQueryRefJson struct {
	RefID       string   `json:"ref_id" yaml:"ref_id" example:"requests"`
	Metric      string   `json:"metric" yaml:"metric" example:"request_events"`
	Aggregation string   `json:"aggregation" yaml:"aggregation" example:"count"`
	GroupBy     []string `json:"group_by,omitempty" yaml:"group_by,omitempty" example:"method,response_status_code"`
}

// MetricsQueryRequestJson is the generic Admin API metrics query request.
//
//	@Description	Generic application metrics query request
type MetricsQueryRequestJson struct {
	Range         MetricsRangeJson      `json:"range" yaml:"range"`
	Namespace     *string               `json:"namespace,omitempty" yaml:"namespace,omitempty" example:"root.**"`
	LabelSelector *string               `json:"label_selector,omitempty" yaml:"label_selector,omitempty" example:"env=prod,team=api"`
	Queries       []MetricsQueryRefJson `json:"queries" yaml:"queries"`
}

// MetricsPointJson is a single bucket value in a metrics series.
//
//	@Description	Single bucket value in a metrics series
type MetricsPointJson struct {
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
	Value     float64   `json:"value" yaml:"value"`
}

// MetricsSeriesJson is one labeled time series returned for a query ref.
//
//	@Description	Labeled metric time series
type MetricsSeriesJson struct {
	RefID       string             `json:"ref_id" yaml:"ref_id"`
	Metric      string             `json:"metric" yaml:"metric"`
	Aggregation string             `json:"aggregation" yaml:"aggregation"`
	Labels      map[string]string  `json:"labels,omitempty" yaml:"labels,omitempty"`
	Points      []MetricsPointJson `json:"points" yaml:"points"`
}

// MetricsQueryResponseJson is the response for a metrics query.
//
//	@Description	Generic application metrics query response
type MetricsQueryResponseJson struct {
	Series []MetricsSeriesJson `json:"series" yaml:"series"`
}

// MetricsSchemaMetricJson describes one metric supported by the metrics query API.
//
//	@Description	Supported metric definition
type MetricsSchemaMetricJson struct {
	Metric       string   `json:"metric" yaml:"metric" example:"request_events"`
	Kind         string   `json:"kind" yaml:"kind" example:"counter"`
	Aggregations []string `json:"aggregations" yaml:"aggregations" example:"count"`
	GroupBy      []string `json:"group_by" yaml:"group_by" example:"method,response_status_code"`
}

// MetricsSchemaResponseJson is the response for the metrics schema endpoint.
//
//	@Description	Application metrics schema response
type MetricsSchemaResponseJson struct {
	Metrics []MetricsSchemaMetricJson `json:"metrics" yaml:"metrics"`
}
