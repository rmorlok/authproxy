package app_metrics

import (
	"context"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

// recordOpts lets tests tweak the synthetic LogRecord they round-trip without
// repeating the full field list each call.
type recordOpts struct {
	timestamp        time.Time
	method           string
	host             string
	path             string
	statusCode       int
	requestType      httpf.RequestType
	correlationId    string
	connectionId     apid.ID
	connectorId      apid.ID
	connectorVersion uint64
	labels           database.Labels
	responseSource   ResponseSource
	rateLimitId      apid.ID
	rateLimitMode    string
	rateLimitBucket  map[string]string
	rateLimitMatched []RateLimitMatch
	reqBodySkipped   BodySkippedReason
	respBodySkipped  BodySkippedReason
}

func makeRecord(namespace string, o recordOpts) *LogRecord {
	if o.timestamp.IsZero() {
		o.timestamp = time.Now().UTC().Truncate(time.Millisecond)
	}
	if o.method == "" {
		o.method = "GET"
	}
	if o.host == "" {
		o.host = "example.com"
	}
	if o.path == "" {
		o.path = "/api/test"
	}
	if o.statusCode == 0 {
		o.statusCode = 200
	}
	if o.requestType == "" {
		o.requestType = httpf.RequestTypeProxy
	}
	return &LogRecord{
		RequestId:           apid.New(apid.PrefixRequestEvents),
		Namespace:           namespace,
		Type:                o.requestType,
		CorrelationId:       o.correlationId,
		Timestamp:           o.timestamp,
		MillisecondDuration: MillisecondDuration(123 * time.Millisecond),
		ConnectionId:        o.connectionId,
		ConnectorId:         o.connectorId,
		ConnectorVersion:    o.connectorVersion,
		Method:              o.method,
		Host:                o.host,
		Scheme:              "https",
		Path:                o.path,
		ResponseStatusCode:  o.statusCode,
		Labels:              o.labels,
		ResponseSource:      o.responseSource,
		RateLimitId:         o.rateLimitId,
		RateLimitMode:       o.rateLimitMode,
		RateLimitBucket:     o.rateLimitBucket,
		RateLimitMatched:    o.rateLimitMatched,
		RequestBodySkipped:  o.reqBodySkipped,
		ResponseBodySkipped: o.respBodySkipped,
	}
}

func TestRequestEvents_PingAndMigrate(t *testing.T) {
	store, _, _ := MustNewBlankRequestEventsStore(t)

	p, ok := store.(pingable)
	require.True(t, ok, "test store should implement pingable")
	require.True(t, p.Ping(context.Background()))
}

func TestRequestEvents_StoreAndGetRecord_RoundTrip(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	rec := makeRecord("root", recordOpts{
		correlationId:    "corr-123",
		connectionId:     apid.New(apid.PrefixConnection),
		connectorId:      apid.New(apid.PrefixConnectorVersion),
		connectorVersion: 7,
		labels:           database.Labels{"env": "prod", "team": "api"},
		responseSource:   ResponseSourceConnectorRateLimiter,
		rateLimitId:      apid.New(apid.PrefixRateLimit),
		rateLimitMode:    "enforce",
		rateLimitBucket:  map[string]string{"path": "/api/test", "method": "GET"},
		rateLimitMatched: []RateLimitMatch{
			{Id: apid.New(apid.PrefixRateLimit), Mode: "observe", Bucket: map[string]string{"k": "v"}},
		},
		reqBodySkipped:  BodySkippedStreaming,
		respBodySkipped: BodySkippedTooLarge,
	})

	require.NoError(t, store.StoreRecord(ctx, rec))

	got, err := retriever.GetRecord(ctx, rec.RequestId)
	require.NoError(t, err)

	require.Equal(t, rec.RequestId, got.RequestId)
	require.Equal(t, rec.Namespace, got.Namespace)
	require.Equal(t, rec.Type, got.Type)
	require.Equal(t, rec.CorrelationId, got.CorrelationId)
	require.True(t, rec.Timestamp.Equal(got.Timestamp))
	require.Equal(t, rec.MillisecondDuration, got.MillisecondDuration)
	require.Equal(t, rec.ConnectionId, got.ConnectionId)
	require.Equal(t, rec.ConnectorId, got.ConnectorId)
	require.Equal(t, rec.ConnectorVersion, got.ConnectorVersion)
	require.Equal(t, rec.Method, got.Method)
	require.Equal(t, rec.Host, got.Host)
	require.Equal(t, rec.Scheme, got.Scheme)
	require.Equal(t, rec.Path, got.Path)
	require.Equal(t, rec.ResponseStatusCode, got.ResponseStatusCode)
	require.Equal(t, rec.Labels, got.Labels)
	require.Equal(t, rec.ResponseSource, got.ResponseSource)
	require.Equal(t, rec.RateLimitId, got.RateLimitId)
	require.Equal(t, rec.RateLimitMode, got.RateLimitMode)
	require.Equal(t, rec.RateLimitBucket, got.RateLimitBucket)
	require.Equal(t, rec.RateLimitMatched, got.RateLimitMatched)
	require.Equal(t, rec.RequestBodySkipped, got.RequestBodySkipped)
	require.Equal(t, rec.ResponseBodySkipped, got.ResponseBodySkipped)
}

func TestRequestEvents_StoreRecords_Batch(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	records := []*LogRecord{
		makeRecord("root", recordOpts{method: "GET", path: "/a"}),
		makeRecord("root", recordOpts{method: "POST", path: "/b"}),
		makeRecord("root", recordOpts{method: "PUT", path: "/c"}),
	}
	require.NoError(t, store.StoreRecords(ctx, records))

	for _, rec := range records {
		got, err := retriever.GetRecord(ctx, rec.RequestId)
		require.NoError(t, err)
		require.Equal(t, rec.Method, got.Method)
		require.Equal(t, rec.Path, got.Path)
	}
}

func TestRequestEvents_StoreRecords_EmptyIsNoop(t *testing.T) {
	store, _, _ := MustNewBlankRequestEventsStore(t)
	require.NoError(t, store.StoreRecords(context.Background(), nil))
	require.NoError(t, store.StoreRecords(context.Background(), []*LogRecord{}))
}

func TestRequestEvents_GetRecord_NotFound(t *testing.T) {
	_, retriever, _ := MustNewBlankRequestEventsStore(t)
	_, err := retriever.GetRecord(context.Background(), apid.New(apid.PrefixRequestEvents))
	require.ErrorIs(t, err, ErrNotFound)
}

// --- list filter coverage ---

func TestRequestEvents_List_FilterByNamespace(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	rRoot := makeRecord("root", recordOpts{})
	rRootA := makeRecord("root.a", recordOpts{})
	rRootAB := makeRecord("root.a.b", recordOpts{})
	rRootC := makeRecord("root.c", recordOpts{})
	require.NoError(t, store.StoreRecords(ctx, []*LogRecord{rRoot, rRootA, rRootAB, rRootC}))

	t.Run("exact namespace match", func(t *testing.T) {
		result := retriever.NewListRequestsBuilder().WithNamespaceMatcher("root.a").FetchPage(ctx)
		require.NoError(t, result.Error)
		ids := collectIDs(result.Results)
		require.Equal(t, map[apid.ID]bool{rRootA.RequestId: true}, ids)
	})

	t.Run("recursive namespace match with .**", func(t *testing.T) {
		result := retriever.NewListRequestsBuilder().WithNamespaceMatcher("root.**").FetchPage(ctx)
		require.NoError(t, result.Error)
		ids := collectIDs(result.Results)
		require.True(t, ids[rRoot.RequestId])
		require.True(t, ids[rRootA.RequestId])
		require.True(t, ids[rRootAB.RequestId])
		require.True(t, ids[rRootC.RequestId])
	})
}

func TestRequestEvents_List_FilterByScalarFields(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	connId := apid.New(apid.PrefixConnection)
	connectorId := apid.New(apid.PrefixConnectorVersion)
	rateLimitId := apid.New(apid.PrefixRateLimit)

	wanted := makeRecord("root", recordOpts{
		correlationId:    "match",
		connectionId:     connId,
		connectorId:      connectorId,
		connectorVersion: 42,
		method:           "POST",
		path:             "/wanted",
		statusCode:       404,
		responseSource:   ResponseSourceConnectorRateLimiter,
		rateLimitId:      rateLimitId,
		requestType:      httpf.RequestTypeOAuth,
	})
	other := makeRecord("root", recordOpts{
		correlationId:    "other",
		connectionId:     apid.New(apid.PrefixConnection),
		connectorId:      apid.New(apid.PrefixConnectorVersion),
		connectorVersion: 1,
		method:           "GET",
		path:             "/other",
		statusCode:       200,
		responseSource:   ResponseSourceUpstream,
		requestType:      httpf.RequestTypeProxy,
	})
	require.NoError(t, store.StoreRecords(ctx, []*LogRecord{wanted, other}))

	cases := []struct {
		name  string
		apply func(b ListRequestBuilder) ListRequestBuilder
	}{
		{"correlation_id", func(b ListRequestBuilder) ListRequestBuilder { return b.WithCorrelationId("match") }},
		{"connection_id", func(b ListRequestBuilder) ListRequestBuilder { return b.WithConnectionId(connId) }},
		{"connector_id", func(b ListRequestBuilder) ListRequestBuilder { return b.WithConnectorId(connectorId) }},
		{"connector_version", func(b ListRequestBuilder) ListRequestBuilder { return b.WithConnectorVersion(42) }},
		{"method", func(b ListRequestBuilder) ListRequestBuilder { return b.WithMethod("POST") }},
		{"status_code single", func(b ListRequestBuilder) ListRequestBuilder { return b.WithStatusCode(404) }},
		{"status_code range", func(b ListRequestBuilder) ListRequestBuilder { return b.WithStatusCodeRangeInclusive(400, 499) }},
		{"path exact", func(b ListRequestBuilder) ListRequestBuilder { return b.WithPath("/wanted") }},
		{"request_type", func(b ListRequestBuilder) ListRequestBuilder { return b.WithRequestType(httpf.RequestTypeOAuth) }},
		{"response_source", func(b ListRequestBuilder) ListRequestBuilder {
			return b.WithResponseSource(ResponseSourceConnectorRateLimiter)
		}},
		{"rate_limit_id", func(b ListRequestBuilder) ListRequestBuilder { return b.WithRateLimitId(rateLimitId) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.apply(retriever.NewListRequestsBuilder()).FetchPage(ctx)
			require.NoError(t, result.Error)
			ids := collectIDs(result.Results)
			require.Equal(t, map[apid.ID]bool{wanted.RequestId: true}, ids,
				"filter %s should return only the wanted record", tc.name)
		})
	}
}

func TestRequestEvents_List_FilterByTimestampRange(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Millisecond)
	old := makeRecord("root", recordOpts{timestamp: base.Add(-2 * time.Hour)})
	mid := makeRecord("root", recordOpts{timestamp: base.Add(-1 * time.Hour)})
	now := makeRecord("root", recordOpts{timestamp: base})
	require.NoError(t, store.StoreRecords(ctx, []*LogRecord{old, mid, now}))

	rangeStart := base.Add(-90 * time.Minute)
	rangeEnd := base.Add(-30 * time.Minute)
	result := retriever.NewListRequestsBuilder().WithTimestampRange(rangeStart, rangeEnd).FetchPage(ctx)
	require.NoError(t, result.Error)
	require.Equal(t, map[apid.ID]bool{mid.RequestId: true}, collectIDs(result.Results))
}

func TestRequestEvents_List_LabelSelector(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	r1 := makeRecord("root", recordOpts{labels: database.Labels{"env": "prod", "team": "api"}})
	r2 := makeRecord("root", recordOpts{labels: database.Labels{"env": "staging", "team": "api"}})
	r3 := makeRecord("root", recordOpts{labels: database.Labels{"env": "prod", "team": "web"}})
	r4 := makeRecord("root", recordOpts{labels: database.Labels{"team": "api"}})
	r5 := makeRecord("root", recordOpts{labels: nil})
	require.NoError(t, store.StoreRecords(ctx, []*LogRecord{r1, r2, r3, r4, r5}))

	t.Run("equality", func(t *testing.T) {
		b, err := retriever.NewListRequestsBuilder().WithLabelSelector("env=prod")
		require.NoError(t, err)
		ids := collectIDs(b.FetchPage(ctx).Results)
		require.Equal(t, map[apid.ID]bool{r1.RequestId: true, r3.RequestId: true}, ids)
	})

	t.Run("multiple equalities", func(t *testing.T) {
		b, err := retriever.NewListRequestsBuilder().WithLabelSelector("env=prod,team=api")
		require.NoError(t, err)
		ids := collectIDs(b.FetchPage(ctx).Results)
		require.Equal(t, map[apid.ID]bool{r1.RequestId: true}, ids)
	})

	t.Run("exists", func(t *testing.T) {
		b, err := retriever.NewListRequestsBuilder().WithLabelSelector("env")
		require.NoError(t, err)
		ids := collectIDs(b.FetchPage(ctx).Results)
		require.Equal(t, map[apid.ID]bool{r1.RequestId: true, r2.RequestId: true, r3.RequestId: true}, ids)
	})

	t.Run("not exists", func(t *testing.T) {
		b, err := retriever.NewListRequestsBuilder().WithLabelSelector("!env")
		require.NoError(t, err)
		ids := collectIDs(b.FetchPage(ctx).Results)
		require.Equal(t, map[apid.ID]bool{r4.RequestId: true, r5.RequestId: true}, ids)
	})

	t.Run("not equal", func(t *testing.T) {
		b, err := retriever.NewListRequestsBuilder().WithLabelSelector("env!=prod")
		require.NoError(t, err)
		ids := collectIDs(b.FetchPage(ctx).Results)
		require.Equal(t, map[apid.ID]bool{r2.RequestId: true, r4.RequestId: true, r5.RequestId: true}, ids)
	})

	t.Run("invalid selector", func(t *testing.T) {
		_, err := retriever.NewListRequestsBuilder().WithLabelSelector("invalid key with spaces=value")
		require.Error(t, err)
	})
}

func TestRequestEvents_List_Ordering(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Millisecond)
	a := makeRecord("root", recordOpts{timestamp: base.Add(-2 * time.Second)})
	b := makeRecord("root", recordOpts{timestamp: base.Add(-1 * time.Second)})
	c := makeRecord("root", recordOpts{timestamp: base})
	require.NoError(t, store.StoreRecords(ctx, []*LogRecord{a, b, c}))

	t.Run("default DESC by timestamp", func(t *testing.T) {
		result := retriever.NewListRequestsBuilder().FetchPage(ctx)
		require.NoError(t, result.Error)
		require.GreaterOrEqual(t, len(result.Results), 3)
		require.Equal(t, c.RequestId, result.Results[0].RequestId)
		require.Equal(t, b.RequestId, result.Results[1].RequestId)
		require.Equal(t, a.RequestId, result.Results[2].RequestId)
	})

	t.Run("explicit ASC by timestamp", func(t *testing.T) {
		result := retriever.NewListRequestsBuilder().
			OrderBy(RequestOrderByTimestamp, pagination.OrderByAsc).
			FetchPage(ctx)
		require.NoError(t, result.Error)
		require.GreaterOrEqual(t, len(result.Results), 3)
		require.Equal(t, a.RequestId, result.Results[0].RequestId)
		require.Equal(t, b.RequestId, result.Results[1].RequestId)
		require.Equal(t, c.RequestId, result.Results[2].RequestId)
	})
}

func TestRequestEvents_List_Pagination_CursorRoundTrip(t *testing.T) {
	store, retriever, _ := MustNewBlankRequestEventsStore(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Millisecond)
	const total = 5
	recs := make([]*LogRecord, total)
	for i := 0; i < total; i++ {
		recs[i] = makeRecord("root", recordOpts{
			timestamp: base.Add(time.Duration(i) * time.Second),
			path:      "/page",
		})
	}
	require.NoError(t, store.StoreRecords(ctx, recs))

	seen := map[apid.ID]bool{}
	addPage := func(p pagination.PageResult[*LogRecord]) {
		for _, r := range p.Results {
			require.False(t, seen[r.RequestId], "duplicate record across pages: %s", r.RequestId)
			seen[r.RequestId] = true
		}
	}

	page1 := retriever.NewListRequestsBuilder().Limit(2).WithPath("/page").FetchPage(ctx)
	require.NoError(t, page1.Error)
	require.True(t, page1.HasMore)
	require.NotEmpty(t, page1.Cursor)
	require.Len(t, page1.Results, 2)
	addPage(page1)

	exec, err := retriever.ListRequestsFromCursor(ctx, page1.Cursor)
	require.NoError(t, err)
	page2 := exec.FetchPage(ctx)
	require.NoError(t, page2.Error)
	addPage(page2)

	// Drain remaining pages if any.
	cursor := page2.Cursor
	for page2.HasMore {
		require.NotEmpty(t, cursor)
		exec, err := retriever.ListRequestsFromCursor(ctx, cursor)
		require.NoError(t, err)
		next := exec.FetchPage(ctx)
		require.NoError(t, next.Error)
		addPage(next)
		cursor = next.Cursor
		page2 = next
	}

	for _, r := range recs {
		require.True(t, seen[r.RequestId], "record %s missing from paginated results", r.RequestId)
	}
}

func collectIDs(records []*LogRecord) map[apid.ID]bool {
	out := make(map[apid.ID]bool, len(records))
	for _, r := range records {
		out[r.RequestId] = true
	}
	return out
}
