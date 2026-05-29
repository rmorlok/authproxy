package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/internal/apgin"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/app_metrics"
	"github.com/rmorlok/authproxy/internal/app_metrics/mock"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/httpf"
	sapi "github.com/rmorlok/authproxy/internal/schema/api"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

func TestRequestEventsRoutes(t *testing.T) {
	type TestSetup struct {
		Gin           *gin.Engine
		AuthUtil      *auth2.AuthTestUtil
		MockRetriever *mock.MockLogRetriever
	}

	setup := func(t *testing.T, cfg config.C) *TestSetup {
		ctrl := gomock.NewController(t)
		cfg, db := database.MustApplyBlankTestDbConfig(t, cfg)
		cfg, auth, authUtil := auth2.TestAuthServiceWithDb(sconfig.ServiceIdApi, cfg, db)

		rlr := mock.NewMockLogRetriever(ctrl)
		rl := NewRequestEventsRoutes(cfg, auth, rlr)

		r := apgin.ForTest(nil)
		rl.Register(r)

		return &TestSetup{
			Gin:           r,
			MockRetriever: rlr,
			AuthUtil:      authUtil,
		}
	}

	t.Run("list", func(t *testing.T) {
		tu := setup(t, nil)

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/metrics/request-events", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "list"), // Wrong resource
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("no results", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*app_metrics.LogRecord]{},
			}

			tu.MockRetriever.EXPECT().
				NewListRequestsBuilder().
				Return(&b)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRequestEventsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 0)
		})

		t.Run("results", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			id := apid.MustParse("req_test550e8400abcde")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*app_metrics.LogRecord]{
					Results: []*app_metrics.LogRecord{
						{
							Namespace:          "root",
							Type:               httpf.RequestTypeProxy,
							RequestId:          id,
							Method:             "GET",
							Path:               "/api/test",
							ResponseStatusCode: 200,
						},
					},
				},
			}

			tu.MockRetriever.EXPECT().
				NewListRequestsBuilder().
				Return(&b)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRequestEventsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, resp.Items[0].RequestId, id)
		})

		t.Run("multiple pages of results", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			id := apid.MustParse("req_test550e8400abcde")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*app_metrics.LogRecord]{
					Results: []*app_metrics.LogRecord{
						{
							Namespace:          "root",
							Type:               httpf.RequestTypeProxy,
							RequestId:          id,
							Method:             "GET",
							Path:               "/api/test",
							ResponseStatusCode: 200,
						},
					},
					Cursor: "next-cursor",
				},
			}

			tu.MockRetriever.EXPECT().
				NewListRequestsBuilder().
				Return(&b)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRequestEventsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, resp.Cursor, "next-cursor")
		})

		t.Run("from cursor", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events?cursor=some-cursor",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			id := apid.MustParse("req_test550e8400abcde")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*app_metrics.LogRecord]{
					Results: []*app_metrics.LogRecord{
						{
							Namespace:          "root",
							Type:               httpf.RequestTypeProxy,
							RequestId:          id,
							Method:             "GET",
							Path:               "/api/test",
							ResponseStatusCode: 200,
						},
					},
				},
			}

			tu.MockRetriever.EXPECT().
				ListRequestsFromCursor(gomock.Any(), "some-cursor").
				Return(&b, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRequestEventsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
		})

		t.Run("bad cursor", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events?cursor=some-cursor",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			cursorError := errors.New("bad cursor")
			b := mock.MockListRequestBuilderExecutor{
				FromCursorError: cursorError, // This is duplicative as it's not actually using the internal from cursor method.
			}

			tu.MockRetriever.EXPECT().
				ListRequestsFromCursor(gomock.Any(), "some-cursor").
				Return(&b, cursorError)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("get", func(t *testing.T) {
		tu := setup(t, nil)
		testId := apid.MustParse("req_test550e8400abcde")
		otherId := apid.MustParse("req_test660e8400abcde")

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/metrics/request-events/"+testId.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "connectors", "get"), // Wrong resource
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("allowed with matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "request-events", "get", testId.String()),
			)
			require.NoError(t, err)

			entry := &app_metrics.FullLog{
				Id:        testId,
				Namespace: "root",
				Timestamp: time.Now().UTC(),
			}

			tu.MockRetriever.EXPECT().
				GetFullLog(gomock.Any(), testId).
				Return(entry, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		t.Run("forbidden with non-matching resource id permission", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "request-events", "get", otherId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "get"),
			)
			require.NoError(t, err)

			tu.MockRetriever.EXPECT().
				GetFullLog(gomock.Any(), testId).
				Return(nil, app_metrics.ErrNotFound)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "get"),
			)
			require.NoError(t, err)

			entry := &app_metrics.FullLog{
				Id:        testId,
				Namespace: "root",
				Timestamp: time.Now().UTC(),
			}

			tu.MockRetriever.EXPECT().
				GetFullLog(gomock.Any(), testId).
				Return(entry, nil)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})
	})

	t.Run("list with label_selector", func(t *testing.T) {
		tu := setup(t, nil)

		t.Run("passes label_selector to builder", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events?label_selector=env%3Dprod%2Cteam%3Dapi",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			id := apid.MustParse("req_test550e8400abcde")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*app_metrics.LogRecord]{
					Results: []*app_metrics.LogRecord{
						{
							Namespace:          "root",
							Type:               httpf.RequestTypeProxy,
							RequestId:          id,
							Method:             "GET",
							Path:               "/api/test",
							ResponseStatusCode: 200,
							Labels:             database.Labels{"env": "prod", "team": "api"},
						},
					},
				},
			}

			tu.MockRetriever.EXPECT().
				NewListRequestsBuilder().
				Return(&b)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRequestEventsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, id, resp.Items[0].RequestId)

			// Verify the label_selector was passed to the builder
			require.NotNil(t, b.LabelSelector)
			require.Equal(t, "env=prod,team=api", *b.LabelSelector)
		})

		t.Run("results include labels in JSON response", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			id := apid.MustParse("req_test550e8400abcde")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*app_metrics.LogRecord]{
					Results: []*app_metrics.LogRecord{
						{
							Namespace:          "root",
							Type:               httpf.RequestTypeProxy,
							RequestId:          id,
							Method:             "GET",
							Path:               "/api/test",
							ResponseStatusCode: 200,
							Labels:             database.Labels{"env": "prod", "region": "us-east"},
						},
					},
				},
			}

			tu.MockRetriever.EXPECT().
				NewListRequestsBuilder().
				Return(&b)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify labels are present in the JSON response
			var raw map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &raw)
			require.NoError(t, err)

			items := raw["items"].([]interface{})
			require.Len(t, items, 1)

			item := items[0].(map[string]interface{})
			labels := item["labels"].(map[string]interface{})
			require.Equal(t, "prod", labels["env"])
			require.Equal(t, "us-east", labels["region"])
		})

		t.Run("results without labels omit labels from JSON", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/metrics/request-events",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-events", "list"),
			)
			require.NoError(t, err)

			id := apid.MustParse("req_test550e8400abcde")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*app_metrics.LogRecord]{
					Results: []*app_metrics.LogRecord{
						{
							Namespace:          "root",
							Type:               httpf.RequestTypeProxy,
							RequestId:          id,
							Method:             "GET",
							Path:               "/api/test",
							ResponseStatusCode: 200,
						},
					},
				},
			}

			tu.MockRetriever.EXPECT().
				NewListRequestsBuilder().
				Return(&b)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			// Verify labels key is omitted when nil
			var raw map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &raw)
			require.NoError(t, err)

			items := raw["items"].([]interface{})
			require.Len(t, items, 1)

			item := items[0].(map[string]interface{})
			_, hasLabels := item["labels"]
			require.False(t, hasLabels, "labels should be omitted from JSON when nil")
		})
	})

	t.Run("query metrics", func(t *testing.T) {
		tu := setup(t, nil)
		start := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
		end := start.Add(time.Hour)

		newMetricsRequest := func(t *testing.T, body string, permissions []aschema.Permission) *http.Request {
			t.Helper()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodPost,
				"/metrics/query",
				bytes.NewBufferString(body),
				"root",
				"some-actor",
				permissions,
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			return req
		}

		validBody := func(overrides map[string]any) string {
			body := map[string]any{
				"range": map[string]any{
					"start": start.Format(time.RFC3339),
					"end":   end.Format(time.RFC3339),
					"step":  "15m",
				},
				"namespace":      "root.tenant.**",
				"label_selector": "env=prod",
				"queries": []map[string]any{
					{
						"ref_id":      "requests",
						"metric":      "request_events",
						"aggregation": "count",
						"group_by":    []string{"method"},
					},
				},
			}
			for k, v := range overrides {
				body[k] = v
			}
			data, err := json.Marshal(body)
			require.NoError(t, err)
			return string(data)
		}

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodPost, "/metrics/query", bytes.NewBufferString("{}"))
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newMetricsRequest(t, validBody(nil), aschema.PermissionsSingle("root.**", "connectors", "list"))

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("executes request event query", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newMetricsRequest(t, validBody(nil), aschema.PermissionsSingle("root.**", "request-events", "list"))

			tu.MockRetriever.EXPECT().
				QueryRequestEventMetrics(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ interface{}, queries []app_metrics.RequestEventMetricsQuery) ([]app_metrics.RequestEventMetricSeries, error) {
					require.Len(t, queries, 1)
					require.Equal(t, "requests", queries[0].RefID)
					require.Equal(t, app_metrics.RequestEventMetricCount, queries[0].Metric)
					require.Equal(t, start, queries[0].Start)
					require.Equal(t, end, queries[0].End)
					require.Equal(t, 15*time.Minute, queries[0].Step)
					require.Equal(t, []string{"root.tenant.**"}, queries[0].NamespaceMatchers)
					require.Equal(t, "env=prod", queries[0].LabelSelector)
					require.Equal(t, []app_metrics.RequestEventGroupBy{app_metrics.RequestEventGroupByMethod}, queries[0].GroupBy)
					return []app_metrics.RequestEventMetricSeries{
						{
							RefID:  "requests",
							Labels: map[string]string{"method": "GET"},
							Points: []app_metrics.RequestEventMetricPoint{{Timestamp: start, Value: 3}},
						},
					}, nil
				})

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp sapi.MetricsQueryResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Series, 1)
			require.Equal(t, "requests", resp.Series[0].RefID)
			require.Equal(t, "request_events", resp.Series[0].Metric)
			require.Equal(t, "count", resp.Series[0].Aggregation)
			require.Equal(t, map[string]string{"method": "GET"}, resp.Series[0].Labels)
			require.Equal(t, 3.0, resp.Series[0].Points[0].Value)
		})

		t.Run("executes resource query", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newMetricsRequest(t, validBody(map[string]any{"queries": []map[string]any{
				{
					"ref_id":      "connections",
					"metric":      "resources.connections",
					"aggregation": "count",
					"group_by":    []string{"state", "health_state"},
				},
			}}), aschema.PermissionsSingle("root.**", "request-events", "list"))

			tu.MockRetriever.EXPECT().
				QueryResourceMetrics(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ interface{}, queries []app_metrics.ResourceMetricsQuery) ([]app_metrics.ResourceMetricSeries, error) {
					require.Len(t, queries, 1)
					require.Equal(t, "connections", queries[0].RefID)
					require.Equal(t, app_metrics.ResourceMetricConnectionsCount, queries[0].Metric)
					require.Equal(t, start, queries[0].Start)
					require.Equal(t, end, queries[0].End)
					require.Equal(t, 15*time.Minute, queries[0].Step)
					require.Equal(t, []string{"root.tenant.**"}, queries[0].NamespaceMatchers)
					require.Equal(t, "env=prod", queries[0].LabelSelector)
					require.Equal(t, []app_metrics.ResourceGroupBy{app_metrics.ResourceGroupByState, app_metrics.ResourceGroupByHealthState}, queries[0].GroupBy)
					return []app_metrics.ResourceMetricSeries{
						{
							RefID:  "connections",
							Labels: map[string]string{"state": "configured", "health_state": "healthy"},
							Points: []app_metrics.ResourceMetricPoint{{Timestamp: start, Value: 2}},
						},
					}, nil
				})

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp sapi.MetricsQueryResponseJson
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Len(t, resp.Series, 1)
			require.Equal(t, "connections", resp.Series[0].RefID)
			require.Equal(t, "resources.connections", resp.Series[0].Metric)
			require.Equal(t, "count", resp.Series[0].Aggregation)
			require.Equal(t, map[string]string{"state": "configured", "health_state": "healthy"}, resp.Series[0].Labels)
			require.Equal(t, 2.0, resp.Series[0].Points[0].Value)
		})

		t.Run("namespace is constrained by actor permissions", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newMetricsRequest(t, validBody(map[string]any{"namespace": "root.**"}), aschema.PermissionsSingle("root.tenant.**", "request-events", "list"))

			tu.MockRetriever.EXPECT().
				QueryRequestEventMetrics(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ interface{}, queries []app_metrics.RequestEventMetricsQuery) ([]app_metrics.RequestEventMetricSeries, error) {
					require.Equal(t, []string{"root.tenant.**"}, queries[0].NamespaceMatchers)
					return nil, nil
				})

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)
		})

		invalidCases := []struct {
			name string
			body string
		}{
			{
				name: "invalid range",
				body: validBody(map[string]any{"range": map[string]any{
					"start": end.Format(time.RFC3339),
					"end":   start.Format(time.RFC3339),
					"step":  "15m",
				}}),
			},
			{
				name: "invalid step",
				body: validBody(map[string]any{"range": map[string]any{
					"start": start.Format(time.RFC3339),
					"end":   end.Format(time.RFC3339),
					"step":  "0m",
				}}),
			},
			{name: "invalid namespace", body: validBody(map[string]any{"namespace": "bad"})},
			{name: "invalid label selector", body: validBody(map[string]any{"label_selector": "bad key=value"})},
			{name: "invalid metric", body: validBody(map[string]any{"queries": []map[string]any{{"ref_id": "x", "metric": "nope", "aggregation": "count"}}})},
			{name: "invalid aggregation", body: validBody(map[string]any{"queries": []map[string]any{{"ref_id": "x", "metric": "request_events", "aggregation": "avg"}}})},
			{name: "invalid group_by", body: validBody(map[string]any{"queries": []map[string]any{{"ref_id": "x", "metric": "request_events", "aggregation": "count", "group_by": []string{"path"}}}})},
			{name: "invalid resource group_by", body: validBody(map[string]any{"queries": []map[string]any{{"ref_id": "x", "metric": "resources.actors", "aggregation": "count", "group_by": []string{"state"}}}})},
			{name: "empty queries", body: validBody(map[string]any{"queries": []map[string]any{}})},
		}

		for _, tc := range invalidCases {
			t.Run(tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				req := newMetricsRequest(t, tc.body, aschema.PermissionsSingle("root.**", "request-events", "list"))

				tu.Gin.ServeHTTP(w, req)
				require.Equal(t, http.StatusBadRequest, w.Code)
			})
		}
	})
}
