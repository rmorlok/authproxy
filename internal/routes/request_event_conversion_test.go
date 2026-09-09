package routes

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	ratelimitschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
	"github.com/stretchr/testify/require"
)

func TestRequestEventToJSONBuildsImmutableProjectionAndReferences(t *testing.T) {
	requestID := apid.MustParse("req_test550e8400abcde")
	actorID := apid.MustParse("act_test550e8400abcde")
	connectionID := apid.MustParse("cxn_test550e8400abcde")
	connectorID := apid.MustParse("cxr_test550e8400abcde")
	rateLimitID := apid.MustParse("rl_test550e8400abcde")
	timestamp := time.Date(2026, 9, 6, 12, 30, 0, 0, time.FixedZone("offset", -5*60*60))
	record := &app_metrics.LogRecord{
		Namespace:           "root.acme",
		Type:                httpf.RequestTypeProxy,
		RequestId:           requestID,
		CorrelationId:       "corr-123",
		Timestamp:           timestamp,
		MillisecondDuration: app_metrics.MillisecondDuration(150 * time.Millisecond),
		ConnectionId:        connectionID,
		ConnectorId:         connectorID,
		ConnectorGeneration: 3,
		Method:              "POST",
		Host:                "api.example.com",
		Scheme:              "https",
		Path:                "/v1/items",
		ResponseStatusCode:  429,
		FullRequestRecorded: true,
		Labels: database.Labels{
			"team":            "payments",
			"apxy/act/-/id":   actorID.String(),
			"apxy/act/-/name": "service-actor",
			"apxy/act/-/ns":   "root.acme",
			"apxy/cxn/-/id":   connectionID.String(),
			"apxy/cxn/-/name": "production",
			"apxy/cxn/-/ns":   "root.acme",
			"apxy/cxr/-/id":   connectorID.String(),
			"apxy/cxr/-/name": "provider",
			"apxy/cxr/-/ns":   "root.integrations",
		},
		ResponseSource:  app_metrics.ResponseSourceRateLimit,
		RateLimitId:     rateLimitID,
		RateLimitMode:   "enforce",
		RateLimitBucket: map[string]string{"actor": actorID.String()},
		RateLimitMatched: []app_metrics.RateLimitMatch{{
			Id:   rateLimitID,
			Mode: "enforce",
		}},
	}

	event := requestEventToJSON(record, nil)
	require.Equal(t, meta.NewTypeMeta("RequestEvent"), event.TypeMeta)
	require.Equal(t, requestID.String(), event.Metadata.ID)
	require.Equal(t, "root.acme", event.Metadata.Namespace)
	require.Equal(t, map[string]string(record.Labels), event.Metadata.Labels)
	require.Equal(t, timestamp.UTC(), *event.Metadata.CreatedAt)
	require.Equal(t, namespaceschema.NamespaceKind, event.Spec.NamespaceRef.Kind)
	require.Equal(t, "root.acme", event.Spec.NamespaceRef.ID)
	require.Equal(t, actorschema.ActorKind, event.Spec.ActorRef.Kind)
	require.Equal(t, actorID.String(), event.Spec.ActorRef.ID)
	require.Equal(t, connectionschema.ConnectionKind, event.Spec.ConnectionRef.Kind)
	require.Equal(t, "production", string(event.Spec.ConnectionRef.Name))
	require.Equal(t, connectorschema.ConnectorKind, event.Spec.ConnectorRef.Kind)
	require.Equal(t, uint64(3), event.Spec.ConnectorRef.Generation)
	require.Equal(t, "root.integrations", event.Spec.ConnectorRef.Namespace)
	require.Equal(t, ratelimitschema.RateLimitKind, event.Spec.RateLimit.RateLimitRef.Kind)
	require.Equal(t, rateLimitID.String(), event.Spec.RateLimit.RateLimitRef.ID)
	require.Len(t, event.Spec.RateLimitsMatched, 1)
	require.True(t, event.Spec.CaptureAvailable)
	require.Nil(t, event.Spec.Capture)
}

func TestRequestEventCaptureUsesSecretReplayRedaction(t *testing.T) {
	record := &app_metrics.LogRecord{
		Namespace:           "root",
		Type:                httpf.RequestTypeProxy,
		RequestId:           apid.MustParse("req_test550e8400abcde"),
		Timestamp:           time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC),
		MillisecondDuration: app_metrics.MillisecondDuration(time.Millisecond),
		Method:              "POST",
		Host:                "api.example.com",
		Scheme:              "https",
		Path:                "/token",
		FullRequestRecorded: true,
	}
	full := &app_metrics.FullLog{
		Full: true,
		Request: app_metrics.FullLogRequest{
			URL:     "https://api.example.com/token?api_key=secret",
			Headers: map[string][]string{"Authorization": {"Bearer secret"}},
			Body:    []byte(`{"client_secret":"secret"}`),
		},
		Response: app_metrics.FullLogResponse{
			Headers: map[string][]string{"Set-Cookie": {"session=secret"}},
			Body:    []byte(`{"access_token":"secret"}`),
		},
	}
	event := requestEventToJSON(record, full)
	require.NotNil(t, event.Spec.Capture)
	require.Equal(t, base64.StdEncoding.EncodeToString(full.Request.Body), event.Spec.Capture.Request.Body)
	require.Equal(t, base64.StdEncoding.EncodeToString(full.Response.Body), event.Spec.Capture.Response.Body)

	redacted, report, err := apserde.MarshalJSONForAPI(context.Background(), event)
	require.NoError(t, err)
	require.True(t, report.Redacted)
	require.NotContains(t, string(redacted), "secret")
	require.Contains(t, string(redacted), "***")

	replayed, report, err := apserde.MarshalJSONForAPI(
		apserde.WithSecretReplay(context.Background(), true),
		event,
	)
	require.NoError(t, err)
	require.False(t, report.Redacted)
	require.Contains(t, string(replayed), "Bearer secret")
	require.Contains(t, string(replayed), "api_key=secret")
}

func TestRequestEventToJSONOmitsUnavailableReferences(t *testing.T) {
	event := requestEventToJSON(&app_metrics.LogRecord{
		Namespace: "root",
		RequestId: apid.MustParse("req_test550e8400abcde"),
	}, nil)
	require.Nil(t, event.Spec.ActorRef)
	require.Nil(t, event.Spec.ConnectionRef)
	require.Nil(t, event.Spec.ConnectorRef)
	require.Nil(t, event.Spec.RateLimit)
}
