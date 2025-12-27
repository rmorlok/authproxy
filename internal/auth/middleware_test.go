package auth

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestActorOnRequest(t *testing.T) {
	t.Parallel()
	assert.False(t, GetAuthFromRequest(util.Must(http.NewRequest("GET", "https://example.com", nil))).IsAuthenticated())

	a := database.Actor{
		ID:         uuid.New(),
		ExternalId: "bobdole",
	}

	r := util.Must(http.NewRequest("GET", "https://example.com", nil))
	r = SetAuthOnRequestContext(r, &RequestAuth{actor: &a})
	assert.Equal(t, &a, GetAuthFromRequest(r).GetActor())
}
