package service

import (
	"net/http"
	"testing"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apauth/core"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestActorOnRequest(t *testing.T) {
	t.Parallel()
	assert.False(t, GetAuthFromRequest(util.Must(http.NewRequest("GET", "https://example.com", nil))).IsAuthenticated())

	a := database.Actor{
		Id:         apid.New(apid.PrefixActor),
		ExternalId: "bobdole",
	}

	r := util.Must(http.NewRequest("GET", "https://example.com", nil))
	r = SetAuthOnRequestContext(r, core.NewAuthenticatedRequestAuth(&a))
	assert.Equal(t, a.Id, GetAuthFromRequest(r).GetActor().GetId())
	assert.Equal(t, a.ExternalId, GetAuthFromRequest(r).GetActor().GetExternalId())
}
