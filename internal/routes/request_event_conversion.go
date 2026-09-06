package routes

import (
	"encoding/base64"
	"fmt"
	"maps"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/database"
	schemaapi "github.com/rmorlok/authproxy/internal/schema/api"
	"github.com/rmorlok/authproxy/internal/schema/common"
	actorschema "github.com/rmorlok/authproxy/internal/schema/resources/actor"
	connectionschema "github.com/rmorlok/authproxy/internal/schema/resources/connection"
	connectorschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	namespaceschema "github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	ratelimitschema "github.com/rmorlok/authproxy/internal/schema/resources/rate_limit"
)

func requestEventToJSON(record *app_metrics.LogRecord, full *app_metrics.FullLog) *schemaapi.RequestEventJson {
	if record == nil {
		return nil
	}

	_, systemLabels := database.SplitUserAndApxyLabels(record.Labels)
	createdAt := record.Timestamp.UTC()
	responseSource := record.ResponseSource
	if responseSource == "" {
		responseSource = app_metrics.ResponseSourceUpstream
	}

	resource := &schemaapi.RequestEventJson{
		TypeMeta: meta.NewTypeMeta(schemaapi.RequestEventKind),
		Metadata: meta.NormalizeObjectMeta(meta.ObjectMeta{
			ID:        record.RequestId.String(),
			Namespace: record.Namespace,
			Labels:    maps.Clone(map[string]string(record.Labels)),
			CreatedAt: &createdAt,
		}),
		Spec: schemaapi.RequestEventSpecJson{
			RequestType:          record.Type,
			CorrelationID:        record.CorrelationId,
			DurationMilliseconds: record.MillisecondDuration.Duration().Milliseconds(),
			NamespaceRef: meta.ObjectReference{
				APIVersion: meta.APIVersionV1Alpha1,
				Kind:       namespaceschema.NamespaceKind,
				ID:         record.Namespace,
			},
			ActorRef: requestEventActorReference(systemLabels),
			ConnectionRef: requestEventResourceReference(
				connectionschema.ConnectionKind,
				record.ConnectionId,
				0,
				systemLabels,
				record.Namespace,
			),
			ConnectorRef: requestEventResourceReference(
				connectorschema.ConnectorKind,
				record.ConnectorId,
				record.ConnectorVersion,
				systemLabels,
				record.Namespace,
			),
			Request: schemaapi.RequestEventRequestJson{
				Method:      record.Method,
				Host:        record.Host,
				Scheme:      record.Scheme,
				Path:        record.Path,
				HTTPVersion: record.RequestHttpVersion,
				SizeBytes:   record.RequestSizeBytes,
				MIMEType:    record.RequestMimeType,
				BodySkipped: string(record.RequestBodySkipped),
			},
			Response: schemaapi.RequestEventResponseJson{
				StatusCode:  record.ResponseStatusCode,
				Error:       record.ResponseError,
				HTTPVersion: record.ResponseHttpVersion,
				SizeBytes:   record.ResponseSizeBytes,
				MIMEType:    record.ResponseMimeType,
				BodySkipped: string(record.ResponseBodySkipped),
				Source:      string(responseSource),
			},
			CaptureAvailable: record.FullRequestRecorded,
			InternalTimeout:  record.InternalTimeout,
			RequestCancelled: record.RequestCancelled,
		},
	}

	if !record.RateLimitId.IsNil() {
		resource.Spec.RateLimit = requestEventRateLimit(
			record.RateLimitId,
			record.RateLimitMode,
			record.RateLimitBucket,
		)
	}
	if len(record.RateLimitMatched) > 0 {
		resource.Spec.RateLimitsMatched = make([]schemaapi.RequestEventRateLimitJson, 0, len(record.RateLimitMatched))
		for _, match := range record.RateLimitMatched {
			resource.Spec.RateLimitsMatched = append(resource.Spec.RateLimitsMatched, *requestEventRateLimit(
				match.Id,
				match.Mode,
				match.Bucket,
			))
		}
	}
	if full != nil && full.Full {
		resource.Spec.Capture = &schemaapi.RequestEventCaptureJson{
			Request: schemaapi.RequestEventCapturedRequestJson{
				URL:     full.Request.URL,
				Headers: cloneHeader(full.Request.Headers),
				Body:    encodeRequestEventBody(full.Request.Body),
			},
			Response: schemaapi.RequestEventCapturedResponseJson{
				Headers: cloneHeader(full.Response.Headers),
				Body:    encodeRequestEventBody(full.Response.Body),
			},
		}
	}

	return resource
}

func requestEventActorReference(labels database.Labels) *meta.ObjectReference {
	actorID := labels[implicitResourceLabel(apid.PrefixActor, "id")]
	id, err := apid.Parse(actorID)
	if err != nil || id.ValidatePrefix(apid.PrefixActor) != nil {
		return nil
	}
	return requestEventResourceReference(actorschema.ActorKind, id, 0, labels, "")
}

func requestEventResourceReference(
	kind meta.Kind,
	id apid.ID,
	generation uint64,
	labels database.Labels,
	fallbackNamespace string,
) *meta.ObjectReference {
	if id.IsNil() {
		return nil
	}
	token := database.ApidPrefixToLabelToken(id.Prefix())
	name := labels[fmt.Sprintf("%s%s/-/name", meta.SystemLabelPrefix, token)]
	namespace := labels[fmt.Sprintf("%s%s/-/ns", meta.SystemLabelPrefix, token)]
	if namespace == "" {
		namespace = fallbackNamespace
	}
	return &meta.ObjectReference{
		APIVersion: meta.APIVersionV1Alpha1,
		Kind:       kind,
		ID:         id.String(),
		Name:       common.ResourceName(name),
		Namespace:  namespace,
		Generation: generation,
	}
}

func requestEventRateLimit(id apid.ID, mode string, bucket map[string]string) *schemaapi.RequestEventRateLimitJson {
	return &schemaapi.RequestEventRateLimitJson{
		RateLimitRef: meta.ObjectReference{
			APIVersion: meta.APIVersionV1Alpha1,
			Kind:       ratelimitschema.RateLimitKind,
			ID:         id.String(),
		},
		Mode:   mode,
		Bucket: maps.Clone(bucket),
	}
}

func implicitResourceLabel(prefix apid.Prefix, field string) string {
	return fmt.Sprintf(
		"%s%s/-/%s",
		meta.SystemLabelPrefix,
		database.ApidPrefixToLabelToken(prefix),
		field,
	)
}

func encodeRequestEventBody(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(value)
}

func cloneHeader(value map[string][]string) map[string][]string {
	if value == nil {
		return make(map[string][]string)
	}
	result := make(map[string][]string, len(value))
	for key, values := range value {
		result[key] = append([]string(nil), values...)
	}
	return result
}
