package plugin

import "time"

type datasourceSettings struct {
	BaseURL string `json:"baseUrl"`
}

type queryModel struct {
	QueryType      string               `json:"queryType"`
	Metric         string               `json:"metric,omitempty"`
	Aggregation    string               `json:"aggregation,omitempty"`
	GroupBy        []string             `json:"groupBy,omitempty"`
	Namespace      string               `json:"namespace,omitempty"`
	LabelSelector  string               `json:"labelSelector,omitempty"`
	RequestFilters requestEventFilters  `json:"requestFilters,omitempty"`
	Variable       variableQueryOptions `json:"variable,omitempty"`
}

type requestEventFilters struct {
	Namespace       string `json:"namespace,omitempty"`
	RequestType     string `json:"requestType,omitempty"`
	CorrelationID   string `json:"correlationId,omitempty"`
	ConnectionID    string `json:"connectionId,omitempty"`
	ConnectorType   string `json:"connectorType,omitempty"`
	ConnectorID     string `json:"connectorId,omitempty"`
	Method          string `json:"method,omitempty"`
	StatusCode      int    `json:"statusCode,omitempty"`
	StatusCodeRange string `json:"statusCodeRange,omitempty"`
	TimestampRange  string `json:"timestampRange,omitempty"`
	Path            string `json:"path,omitempty"`
	PathRegex       string `json:"pathRegex,omitempty"`
	LabelSelector   string `json:"labelSelector,omitempty"`
	ResponseSource  string `json:"responseSource,omitempty"`
	RateLimitID     string `json:"rateLimitId,omitempty"`
}

type variableQueryOptions struct {
	Type          string `json:"type"`
	Namespace     string `json:"namespace,omitempty"`
	LabelSelector string `json:"labelSelector,omitempty"`
	ConnectorID   string `json:"connectorId,omitempty"`
}

type metricsQueryRequest struct {
	Range         metricsRange      `json:"range"`
	Namespace     *string           `json:"namespace,omitempty"`
	LabelSelector *string           `json:"label_selector,omitempty"`
	Queries       []metricsQueryRef `json:"queries"`
}

type metricsRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Step  string    `json:"step"`
}

type metricsQueryRef struct {
	RefID       string   `json:"ref_id"`
	Metric      string   `json:"metric"`
	Aggregation string   `json:"aggregation"`
	GroupBy     []string `json:"group_by,omitempty"`
}

type metricsQueryResponse struct {
	Series []metricsSeries `json:"series"`
}

type metricsSeries struct {
	RefID       string            `json:"ref_id"`
	Metric      string            `json:"metric"`
	Aggregation string            `json:"aggregation"`
	Labels      map[string]string `json:"labels,omitempty"`
	Points      []metricsPoint    `json:"points"`
}

type metricsPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type listResponse[T any] struct {
	Items  []T    `json:"items"`
	Cursor string `json:"cursor,omitempty"`
	Total  *int64 `json:"total,omitempty"`
}

type namedResource struct {
	ID          string            `json:"id"`
	Path        string            `json:"path"`
	ExternalID  string            `json:"external_id"`
	DisplayName string            `json:"display_name"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Connector   *namedResource    `json:"connector,omitempty"`
}

type requestEvent struct {
	Namespace           string            `json:"namespace"`
	Type                string            `json:"type"`
	RequestID           string            `json:"request_id"`
	CorrelationID       string            `json:"correlation_id"`
	Timestamp           time.Time         `json:"timestamp"`
	MillisecondDuration int64             `json:"duration"`
	ConnectionID        string            `json:"connection_id"`
	ConnectorID         string            `json:"connector_id"`
	ConnectorVersion    uint64            `json:"connector_version"`
	Method              string            `json:"method"`
	Host                string            `json:"host"`
	Scheme              string            `json:"scheme"`
	Path                string            `json:"path"`
	RequestSizeBytes    int64             `json:"request_size_bytes"`
	ResponseStatusCode  int               `json:"response_status_code"`
	ResponseError       string            `json:"response_error"`
	ResponseSizeBytes   int64             `json:"response_size_bytes"`
	InternalTimeout     bool              `json:"internal_timeout"`
	RequestCancelled    bool              `json:"request_cancelled"`
	Labels              map[string]string `json:"labels,omitempty"`
	ResponseSource      string            `json:"response_source"`
	RateLimitID         string            `json:"rate_limit_id"`
	RateLimitMode       string            `json:"rate_limit_mode"`
	RateLimitBucket     map[string]string `json:"rate_limit_bucket,omitempty"`
	FullRequestRecorded bool              `json:"full_request_recorded"`
	RequestBodySkipped  string            `json:"request_body_skipped"`
	ResponseBodySkipped string            `json:"response_body_skipped"`
	RequestHTTPVersion  string            `json:"request_http_version"`
	ResponseHTTPVersion string            `json:"response_http_version"`
	RequestMimeType     string            `json:"request_mime_type"`
	ResponseMimeType    string            `json:"response_mime_type"`
}
