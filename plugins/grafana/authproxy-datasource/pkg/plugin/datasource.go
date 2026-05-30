package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	queryTypeMetrics       = "metrics"
	queryTypeRequestEvents = "request_events"
	queryTypeVariable      = "variable"
)

type Datasource struct {
	client *authProxyClient
}

var (
	_ backend.QueryDataHandler   = (*Datasource)(nil)
	_ backend.CheckHealthHandler = (*Datasource)(nil)
	_ instancemgmt.Instance      = (*Datasource)(nil)
)

func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	var cfg datasourceSettings
	if len(settings.JSONData) > 0 {
		if err := json.Unmarshal(settings.JSONData, &cfg); err != nil {
			return nil, fmt.Errorf("parse datasource settings: %w", err)
		}
	}
	jwt := settings.DecryptedSecureJSONData["jwt"]
	client, err := newAuthProxyClient(cfg.BaseURL, jwt, http.DefaultClient)
	if err != nil {
		return nil, err
	}
	return &Datasource{client: client}, nil
}

func (d *Datasource) Dispose() {}

func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if err := d.client.health(ctx); err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: err.Error(),
		}, nil
	}
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "AuthProxy metrics API reachable",
	}, nil
}

func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	resp := backend.NewQueryDataResponse()
	for _, q := range req.Queries {
		resp.Responses[q.RefID] = d.query(ctx, q)
	}
	return resp, nil
}

func (d *Datasource) query(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	var qm queryModel
	if err := json.Unmarshal(q.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("invalid query JSON: %v", err))
	}

	switch qm.QueryType {
	case "", queryTypeMetrics:
		return d.queryMetrics(ctx, q, qm)
	case queryTypeRequestEvents:
		return d.queryRequestEvents(ctx, qm)
	case queryTypeVariable:
		return d.queryVariable(ctx, qm)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unsupported query type %q", qm.QueryType))
	}
}

func (d *Datasource) queryMetrics(ctx context.Context, q backend.DataQuery, qm queryModel) backend.DataResponse {
	if strings.TrimSpace(qm.Metric) == "" || strings.TrimSpace(qm.Aggregation) == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "metric and aggregation are required")
	}

	step := q.Interval
	if step <= 0 {
		step = 15 * time.Minute
	}
	apiReq := metricsQueryRequest{
		Range: metricsRange{
			Start: q.TimeRange.From,
			End:   q.TimeRange.To,
			Step:  step.String(),
		},
		Queries: []metricsQueryRef{{
			RefID:       q.RefID,
			Metric:      qm.Metric,
			Aggregation: qm.Aggregation,
			GroupBy:     qm.GroupBy,
		}},
	}
	if qm.Namespace != "" {
		apiReq.Namespace = &qm.Namespace
	}
	if qm.LabelSelector != "" {
		apiReq.LabelSelector = &qm.LabelSelector
	}

	apiResp, err := d.client.queryMetrics(ctx, apiReq)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadGateway, err.Error())
	}

	frames := data.Frames{}
	for _, series := range apiResp.Series {
		times := make([]time.Time, 0, len(series.Points))
		values := make([]float64, 0, len(series.Points))
		for _, point := range series.Points {
			times = append(times, point.Timestamp)
			values = append(values, point.Value)
		}
		frame := data.NewFrame(series.RefID,
			data.NewField("time", nil, times),
			data.NewField(series.Metric+"."+series.Aggregation, data.Labels(series.Labels), values),
		)
		frames = append(frames, frame)
	}
	return backend.DataResponse{Frames: frames}
}

func (d *Datasource) queryRequestEvents(ctx context.Context, qm queryModel) backend.DataResponse {
	if qm.RequestFilters.Namespace == "" {
		qm.RequestFilters.Namespace = qm.Namespace
	}
	if qm.RequestFilters.LabelSelector == "" {
		qm.RequestFilters.LabelSelector = qm.LabelSelector
	}
	events, err := d.client.listRequestEvents(ctx, qm.RequestFilters)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadGateway, err.Error())
	}
	return backend.DataResponse{Frames: data.Frames{requestEventsFrame(events)}}
}

func (d *Datasource) queryVariable(ctx context.Context, qm queryModel) backend.DataResponse {
	values, err := d.client.variableValues(ctx, qm.Variable)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadGateway, err.Error())
	}
	text := make([]string, 0, len(values))
	value := make([]string, 0, len(values))
	for _, item := range values {
		value = append(value, resourceValue(item))
		text = append(text, resourceText(item))
	}
	return backend.DataResponse{Frames: data.Frames{
		data.NewFrame("variables",
			data.NewField("text", nil, text),
			data.NewField("value", nil, value),
		),
	}}
}

func requestEventsFrame(events []requestEvent) *data.Frame {
	timestamp := make([]time.Time, 0, len(events))
	namespace := make([]string, 0, len(events))
	requestID := make([]string, 0, len(events))
	correlationID := make([]string, 0, len(events))
	method := make([]string, 0, len(events))
	pathVals := make([]string, 0, len(events))
	status := make([]int64, 0, len(events))
	duration := make([]int64, 0, len(events))
	connectionID := make([]string, 0, len(events))
	connectorID := make([]string, 0, len(events))
	responseSource := make([]string, 0, len(events))
	rateLimitID := make([]string, 0, len(events))
	labels := make([]string, 0, len(events))

	for _, event := range events {
		timestamp = append(timestamp, event.Timestamp)
		namespace = append(namespace, event.Namespace)
		requestID = append(requestID, event.RequestID)
		correlationID = append(correlationID, event.CorrelationID)
		method = append(method, event.Method)
		pathVals = append(pathVals, event.Path)
		status = append(status, int64(event.ResponseStatusCode))
		duration = append(duration, event.MillisecondDuration)
		connectionID = append(connectionID, event.ConnectionID)
		connectorID = append(connectorID, event.ConnectorID)
		responseSource = append(responseSource, event.ResponseSource)
		rateLimitID = append(rateLimitID, event.RateLimitID)
		labels = append(labels, stableJSON(event.Labels))
	}

	return data.NewFrame("request_events",
		data.NewField("timestamp", nil, timestamp),
		data.NewField("namespace", nil, namespace),
		data.NewField("request_id", nil, requestID),
		data.NewField("correlation_id", nil, correlationID),
		data.NewField("method", nil, method),
		data.NewField("path", nil, pathVals),
		data.NewField("status", nil, status),
		data.NewField("duration_ms", nil, duration),
		data.NewField("connection_id", nil, connectionID),
		data.NewField("connector_id", nil, connectorID),
		data.NewField("response_source", nil, responseSource),
		data.NewField("rate_limit_id", nil, rateLimitID),
		data.NewField("labels", nil, labels),
	)
}

func resourceValue(item namedResource) string {
	switch {
	case item.ID != "":
		return item.ID
	case item.Path != "":
		return item.Path
	case item.ExternalID != "":
		return item.ExternalID
	default:
		return item.Name
	}
}

func resourceText(item namedResource) string {
	switch {
	case item.DisplayName != "":
		return item.DisplayName
	case item.Name != "":
		return item.Name
	case item.Path != "":
		return item.Path
	case item.ExternalID != "":
		return item.ExternalID
	default:
		return item.ID
	}
}

func stableJSON(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		k, _ := json.Marshal(key)
		v, _ := json.Marshal(labels[key])
		parts = append(parts, string(k)+":"+string(v))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
