package routes

import (
	"encoding/json"
	"errors"
	"github.com/rmorlok/authproxy/internal/api_common"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	auth2 "github.com/rmorlok/authproxy/internal/apauth/service"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/request_log"
	"github.com/rmorlok/authproxy/internal/request_log/mock"
	aschema "github.com/rmorlok/authproxy/internal/schema/auth"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

func TestRequestLogRoutes(t *testing.T) {
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
		rl := NewRequestLogRoutes(cfg, auth, rlr)

		r := api_common.GinForTest(nil)
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
			req, err := http.NewRequest(http.MethodGet, "/request-log", nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log",
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
				"/request-log",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-log", "list"),
			)
			require.NoError(t, err)

			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*request_log.EntryRecord]{},
			}

			tu.MockRetriever.EXPECT().
				NewListRequestsBuilder().
				Return(&b)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp ListRequestsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 0)
		})

		t.Run("results", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-log", "list"),
			)
			require.NoError(t, err)

			id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*request_log.EntryRecord]{
					Results: []*request_log.EntryRecord{
						{
							Namespace:          "root",
							Type:               request_log.RequestTypeProxy,
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

			var resp ListRequestsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, resp.Items[0].RequestId, id)
		})

		t.Run("multiple pages of results", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-log", "list"),
			)
			require.NoError(t, err)

			id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*request_log.EntryRecord]{
					Results: []*request_log.EntryRecord{
						{
							Namespace:          "root",
							Type:               request_log.RequestTypeProxy,
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

			var resp ListRequestsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
			require.Equal(t, resp.Cursor, "next-cursor")
		})

		t.Run("from cursor", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log?cursor=some-cursor",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-log", "list"),
			)
			require.NoError(t, err)

			id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
			b := mock.MockListRequestBuilderExecutor{
				ReturnResults: pagination.PageResult[*request_log.EntryRecord]{
					Results: []*request_log.EntryRecord{
						{
							Namespace:          "root",
							Type:               request_log.RequestTypeProxy,
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

			var resp ListRequestsResponseJson
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			require.Len(t, resp.Items, 1)
		})

		t.Run("bad cursor", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log?cursor=some-cursor",
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-log", "list"),
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
		testId := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		otherId := uuid.MustParse("660e8400-e29b-41d4-a716-446655440001")

		t.Run("unauthorized", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/request-log/"+testId.String(), nil)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("forbidden", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log/"+testId.String(),
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
				"/request-log/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "request-log", "get", testId.String()),
			)
			require.NoError(t, err)

			entry := &request_log.Entry{
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
				"/request-log/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingleWithResourceIds("root.**", "request-log", "get", otherId.String()),
			)
			require.NoError(t, err)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusForbidden, w.Code)
		})

		t.Run("not found", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-log", "get"),
			)
			require.NoError(t, err)

			tu.MockRetriever.EXPECT().
				GetFullLog(gomock.Any(), testId).
				Return(nil, request_log.ErrNotFound)

			tu.Gin.ServeHTTP(w, req)
			require.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("valid", func(t *testing.T) {
			w := httptest.NewRecorder()
			req, err := tu.AuthUtil.NewSignedRequestForActorExternalId(
				http.MethodGet,
				"/request-log/"+testId.String(),
				nil,
				"root",
				"some-actor",
				aschema.PermissionsSingle("root.**", "request-log", "get"),
			)
			require.NoError(t, err)

			entry := &request_log.Entry{
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
}
