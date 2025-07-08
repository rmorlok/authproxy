package database

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/apctx"
	"github.com/rmorlok/authproxy/test_utils"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	clock "k8s.io/utils/clock/testing"
	"testing"
	"time"
)

func TestConnections(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		_, db := MustApplyBlankTestDbConfig("connection_round_trip", nil)
		now := time.Date(1955, time.November, 5, 6, 29, 0, 0, time.UTC)
		ctx := apctx.NewBuilderBackground().WithClock(clock.NewFakeClock(now)).Build()

		u := uuid.New()
		err := db.CreateConnection(ctx, &Connection{ID: u, State: ConnectionStateCreated})
		assert.NoError(t, err)

		c, err := db.GetConnection(ctx, u)
		assert.NoError(t, err)
		assert.NotNil(t, c)
		assert.Equal(t, c.ID, u)
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
			Id    string
			State string
			gorm.DeletedAt
		}

		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, deleted_at FROM connections;
		`, []connectionResult{})

		u := uuid.New()
		err := db.CreateConnection(ctx, &Connection{ID: u, State: ConnectionStateCreated})
		assert.NoError(t, err)

		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, deleted_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateCreated),
				DeletedAt: gorm.DeletedAt{},
			},
		})

		// Delete a connection that does not exist
		err = db.DeleteConnection(ctx, uuid.New())
		assert.NoError(t, err)

		// Unchanged
		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, deleted_at FROM connections;
		`, []connectionResult{
			{
				Id:        u.String(),
				State:     string(ConnectionStateCreated),
				DeletedAt: gorm.DeletedAt{},
			},
		})

		err = db.DeleteConnection(ctx, u)
		assert.NoError(t, err)

		test_utils.AssertSql(t, rawDb, `
			SELECT id,state, deleted_at FROM connections;
		`, []connectionResult{
			{
				Id:    u.String(),
				State: string(ConnectionStateCreated),
				DeletedAt: gorm.DeletedAt{
					Time:  now,
					Valid: true,
				},
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

			err := db.CreateConnection(ctx, &Connection{ID: u, State: state})
			assert.NoError(t, err)
		}

		t.Run("all connections", func(t *testing.T) {
			result := db.ListConnectionsBuilder().Limit(10).FetchPage(ctx)
			assert.NoError(t, result.Error)
			assert.Len(t, result.Results, 10)
			assert.Equal(t, result.Results[0].ID, firstUuid)
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
			assert.Equal(t, lastUuid, last.ID)
		})

		t.Run("filter by state", func(t *testing.T) {
			total := 0

			q := db.ListConnectionsBuilder().Limit(10).ForConnectionState(ConnectionStateReady)
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
			q := db.ListConnectionsBuilder().Limit(7).OrderBy(ConnectionOrderByCreatedAt, OrderByDesc)
			err := q.Enumerate(ctx, func(result PageResult[Connection]) (bool, error) {
				allResults = append(allResults, result.Results...)
				return true, nil
			})

			assert.NoError(t, err)
			assert.Len(t, allResults, 50)
			assert.Equal(t, lastUuid, allResults[0].ID)
			assert.Equal(t, firstUuid, allResults[49].ID)
		})
	})
}
