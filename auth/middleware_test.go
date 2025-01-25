package auth

import (
	"github.com/google/uuid"
	"github.com/rmorlok/authproxy/database"
	"github.com/rmorlok/authproxy/util"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func TestActorOnRequest(t *testing.T) {
	assert.False(t, GetAuthFromRequest(util.Must(http.NewRequest("GET", "https://example.com", nil))).IsAuthenticated())

	a := database.Actor{
		ID:         uuid.New(),
		ExternalId: "bobdole",
	}

	r := util.Must(http.NewRequest("GET", "https://example.com", nil))
	r = SetAuthOnRequestContext(r, &requestAuth{actor: &a})
	assert.Equal(t, &a, GetAuthFromRequest(r).GetActor())
}
