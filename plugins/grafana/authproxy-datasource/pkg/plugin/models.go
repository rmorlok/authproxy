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
	LabelSelector *string           `json:"labelSelector,omitempty"`
	Queries       []metricsQueryRef `json:"queries"`
}

type metricsRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Step  string    `json:"step"`
}

type metricsQueryRef struct {
	RefID       string   `json:"refId"`
	Metric      string   `json:"metric"`
	Aggregation string   `json:"aggregation"`
	GroupBy     []string `json:"groupBy,omitempty"`
}

type metricsQueryResponse struct {
	Series []metricsSeries `json:"series"`
}

type metricsSeries struct {
	RefID       string            `json:"refId"`
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

type resourceMetadata struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type resourceSpec struct {
	ExternalID string `json:"externalId"`
}

type namedResource struct {
	APIVersion  string            `json:"apiVersion"`
	Kind        string            `json:"kind"`
	Metadata    *resourceMetadata `json:"metadata,omitempty"`
	Spec        *resourceSpec     `json:"spec,omitempty"`
	ID          string            `json:"id"`
	Path        string            `json:"path"`
	ExternalID  string            `json:"externalId"`
	DisplayName string            `json:"displayName"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels,omitempty"`
	Connector   *namedResource    `json:"connector,omitempty"`
}

type requestEvent struct {
	Namespace           string            `json:"namespace"`
	Type                string            `json:"type"`
	RequestID           string            `json:"requestId"`
	CorrelationID       string            `json:"correlationId"`
	Timestamp           time.Time         `json:"timestamp"`
	MillisecondDuration int64             `json:"duration"`
	ConnectionID        string            `json:"connectionId"`
	ConnectorID         string            `json:"connectorId"`
	ConnectorVersion    uint64            `json:"connectorVersion"`
	Method              string            `json:"method"`
	Host                string            `json:"host"`
	Scheme              string            `json:"scheme"`
	Path                string            `json:"path"`
	RequestSizeBytes    int64             `json:"requestSizeBytes"`
	ResponseStatusCode  int               `json:"responseStatusCode"`
	ResponseError       string            `json:"responseError"`
	ResponseSizeBytes   int64             `json:"responseSizeBytes"`
	InternalTimeout     bool              `json:"internalTimeout"`
	RequestCancelled    bool              `json:"requestCancelled"`
	Labels              map[string]string `json:"labels,omitempty"`
	ResponseSource      string            `json:"responseSource"`
	RateLimitID         string            `json:"rateLimitId"`
	RateLimitMode       string            `json:"rateLimitMode"`
	RateLimitBucket     map[string]string `json:"rateLimitBucket,omitempty"`
	FullRequestRecorded bool              `json:"fullRequestRecorded"`
	RequestBodySkipped  string            `json:"requestBodySkipped"`
	ResponseBodySkipped string            `json:"responseBodySkipped"`
	RequestHTTPVersion  string            `json:"requestHttpVersion"`
	ResponseHTTPVersion string            `json:"responseHttpVersion"`
	RequestMimeType     string            `json:"requestMimeType"`
	ResponseMimeType    string            `json:"responseMimeType"`
}
