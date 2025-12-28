package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/apctx"
	"github.com/rmorlok/authproxy/internal/test_utils"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/assert"
	clock "k8s.io/utils/clock/testing"
)

func TestConnections(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("connection_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		u := uuid.New()
		err := db.CreateConnection(ctx, &Connection{
			Id:               u,
			Namespace:        "root.some-namespace",
			ConnectorId:      uuid.New(),
			ConnectorVersion: 1,
			State:            ConnectionStateCreated,
		})
		assert.NoError(t, err)

		c, err := db.GetConnection(ctx, u)
		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.Equal(t, c.Id, u)
		assert.Equal(t, c.State, ConnectionStateCreated)
		assert.Equal(t, now, c.CreatedAt)
		assert.Equal(t, now, c.UpdatedAt)
	})
	t.Run("delete connection", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw("delete_connection", nil)
		defer rawDb.Close()
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		type connectionResult struct {
			Id        string
			State     string
			DeletedAt *time.Time
		}

		test_utils.AssertSql(t, rawDb, `
			SELECT id, state, deleted_at FROM connections;
		`, []connectionResult{})

		u := uuid.New()
		err := db.CreateConnection(ctx, &Connection{
			Id:               u,
			Namespace:        "root.some-namespace",
			ConnectorId:      uuid.New(),
			ConnectorVersion: 1,
			State:            ConnectionStateCreated})
		assert.NoError(t, err)

		test_utils.AssertSql(t, rawDb, `
			SELECT id, state, deleted_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateCreated),
				DeletedAt: nil,
			},
		})

		// Delete a connection that does not exist
		err = db.DeleteConnection(ctx, uuid.New())
		assert.ErrorIs(t, err, ErrNotFound)

		// Unchanged
		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, deleted_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateCreated),
				DeletedAt: nil,
			},
		})

		err = db.DeleteConnection(ctx, u)
		assert.NoError(t, err)

		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, deleted_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateCreated),
				DeletedAt: &now,
			},
		})
	})
	t.Run("set connection state", func(t *testing.T) {
		_, db, rawDb := MustApplyBlankTestDbConfigRaw("set_connection_state", nil)
		defer rawDb.Close()
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		type connectionResult struct {
			Id        string
			State     string
			UpdatedAt time.Time
		}

		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, updated_at FROM connections;
		`, []connectionResult{})

		u := uuid.New()
		err := db.CreateConnection(ctx, &Connection{
			Id:               u,
			Namespace:        "root.some-namespace",
			ConnectorId:      uuid.New(),
			ConnectorVersion: 1,
			State:            ConnectionStateCreated,
		})
		assert.NoError(t, err)

		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, updated_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateCreated),
				UpdatedAt: now,
			},
		})

		newNow := time.Date(1955, time.November, 6, 6, 29, 0, 0, time.UTC)
		ctx = apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(newNow)).Build()

		// Attempt update for connection that does not exist
		err = db.SetConnectionState(ctx, uuid.New(), ConnectionStateReady)
		assert.ErrorIs(t, err, ErrNotFound)

		// Unchanged
		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, updated_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateCreated),
				UpdatedAt: now,
			},
		})

		err = db.SetConnectionState(ctx, u, ConnectionStateReady)
		assert.NoError(t, err)

		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, updated_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateReady),
				UpdatedAt: newNow,
			},
		})
	})

	t.Run("list connections", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("connection_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		c := clock.NewFakeClock(now)
		ctx := apctx.NewBuilderBackground().WithClock(c).Build()

		var firstUuid, lastUuid uuid.UUID
		for i := 0; i < 50; i++ {
			now = now.Add(time.Second)
			c.SetTime(now)

			u := uuid.New()
			if i == 0 {
				firstUuid = u
			}
			lastUuid = u

			state := ConnectionStateCreated
			if i%2 == 1 {
				state = ConnectionStateReady
			}

			err := db.CreateConnection(ctx, &Connection{
				Id:               u,
				Namespace:        fmt.Sprintf("root.some-namespace.%d", i%10),
				ConnectorId:      uuid.New(),
				ConnectorVersion: 1,
				State:            state,
			})
			assert.NoError(t, err)
		}

		t.Run("all connections", func(t *testing.T) {
			result := db.ListConnectionsBuilder().Limit(10).FetchPage(ctx)
			assert.NoError(t, result.Error)
			assert.Len(t, result.Results, 10)
			assert.Equal(t, result.Results[0].Id, firstUuid)
			assert.True(t, result.HasMore)
			assert.NotEmpty(t, result.Cursor)

			total := 10
			cursor := result.Cursor
			var last Connection

			for cursor != "" {
				ex, err := db.ListConnectionsFromCursor(ctx, cursor)
				assert.NoError(t, err)
				result = ex.FetchPage(ctx)
				assert.NoError(t, result.Error)
				assert.True(t, len(result.Results) > 0)

				last = result.Results[len(result.Results)-1]
				total += len(result.Results)
				cursor = result.Cursor
			}

			assert.Equal(t, 50, total)
			assert.Equal(t, lastUuid, last.Id)
		})

		t.Run("filter by namespace", func(t *testing.T) {
			result := db.ListConnectionsBuilder().
				ForNamespaceMatcher("root.some-namespace.0").
				OrderBy(ConnectionOrderByCreatedAt, pagination.OrderByAsc).
				Limit(51).
				FetchPage(ctx)
			assert.NoError(t, result.Error)
			assert.Len(t, result.Results, 5)
			assert.Equal(t, result.Results[0].Id, firstUuid)
			assert.False(t, result.HasMore)
			assert.Empty(t, result.Cursor)

			result = db.ListConnectionsBuilder().
				ForNamespaceMatcher("root.some-namespace.2").
				OrderBy(ConnectionOrderByCreatedAt, pagination.OrderByAsc).
				Limit(51).
				FetchPage(ctx)
			assert.NoError(t, result.Error)
			assert.Len(t, result.Results, 5)
			assert.NotEqual(t, result.Results[0].Id, firstUuid)
			assert.False(t, result.HasMore)
			assert.Empty(t, result.Cursor)
		})

		t.Run("filter by state", func(t *testing.T) {
			total := 0

			q := db.ListConnectionsBuilder().Limit(10).ForState(ConnectionStateReady)
			for {
				result := q.FetchPage(ctx)
				assert.NoError(t, result.Error)
				total += len(result.Results)
				if result.Error != nil || !result.HasMore {
					break
				}
			}

			assert.Equal(t, 25, total)
		})

		t.Run("reverse order", func(t *testing.T) {
			var allResults []Connection
			q := db.
				ListConnectionsBuilder().
				Limit(7).
				OrderBy(ConnectionOrderByCreatedAt, pagination.OrderByDesc)
			err := q.Enumerate(ctx, func(result pagination.PageResult[Connection]) (bool, error) {
				allResults = append(allResults, result.Results...)
				return true, nil
			})

			assert.NoError(t, err)
			assert.Len(t, allResults, 50)
			assert.Equal(t, lastUuid, allResults[0].Id)
			assert.Equal(t, firstUuid, allResults[49].Id)
		})
	})

	t.Run("enumerate connections", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("enumerate_connections", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		c := clock.NewFakeClock(now)
		ctx := apctx.NewBuilderBackground().WithClock(c).Build()

		for i := 0; i < 201; i++ {
			now = now.Add(time.Second)
			c.SetTime(now)

			u := uuid.New()

			state := ConnectionStateCreated
			if i%2 == 1 {
				state = ConnectionStateReady
			}

			err := db.CreateConnection(ctx, &Connection{
				Id:               u,
				Namespace:        "root.some-namespace",
				ConnectorId:      uuid.New(),
				ConnectorVersion: 1,
				State:            state,
			})
			assert.NoError(t, err)
		}

		// A disconnecting connection
		now = now.Add(time.Second)
		c.SetTime(now)
		u := uuid.New()
		err := db.CreateConnection(ctx, &Connection{
			Id:               u,
			Namespace:        "root.some-namespace",
			ConnectorId:      uuid.New(),
			ConnectorVersion: 1,
			State:            ConnectionStateDisconnecting,
		})
		assert.NoError(t, err)

		// A deleted connection
		now = now.Add(time.Second)
		c.SetTime(now)
		u = uuid.New()
		err = db.CreateConnection(ctx, &Connection{
			Id:               u,
			Namespace:        "root.some-namespace",
			ConnectorId:      uuid.New(),
			ConnectorVersion: 1,
			State:            ConnectionStateDisconnected,
		})
		assert.NoError(t, err)
		now = now.Add(time.Second)
		c.SetTime(now)
		err = db.DeleteConnection(ctx, u)
		assert.NoError(t, err)

		t.Run("all connections", func(t *testing.T) {
			total := 0
			err := db.
				ListConnectionsBuilder().
				WithDeletedHandling(DeletedHandlingInclude).
				Enumerate(ctx, func(pr pagination.PageResult[Connection]) (keepGoing bool, err error) {
					total += len(pr.Results)
					return true, nil
				})
			assert.NoError(t, err)
			assert.Equal(t, 203, total)
		})
		t.Run("not deleted", func(t *testing.T) {
			total := 0
			err := db.
				ListConnectionsBuilder().
				WithDeletedHandling(DeletedHandlingExclude).
				Enumerate(ctx, func(pr pagination.PageResult[Connection]) (keepGoing bool, err error) {
					total += len(pr.Results)
					return true, nil
				})
			assert.NoError(t, err)
			assert.Equal(t, 202, total)
		})
		t.Run("multiple state filter", func(t *testing.T) {
			total := 0
			err := db.
				ListConnectionsBuilder().
				ForStates([]ConnectionState{ConnectionStateCreated, ConnectionStateReady}).
				WithDeletedHandling(DeletedHandlingExclude).
				Enumerate(ctx, func(pr pagination.PageResult[Connection]) (keepGoing bool, err error) {
					total += len(pr.Results)
					return true, nil
				})
			assert.NoError(t, err)
			assert.Equal(t, 201, total)
		})
		t.Run("single state filter", func(t *testing.T) {
			total := 0
			err := db.
				ListConnectionsBuilder().
				ForStates([]ConnectionState{ConnectionStateReady}).
				WithDeletedHandling(DeletedHandlingExclude).
				Enumerate(ctx, func(pr pagination.PageResult[Connection]) (keepGoing bool, err error) {
					total += len(pr.Results)
					return false, nil
				})
			assert.NoError(t, err)
			assert.Equal(t, 100, total)
		})
	})
}
