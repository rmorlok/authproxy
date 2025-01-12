package auth

import (
	"github.com/rmorlok/authproxy/jwt"
	"github.com/rmorlok/authproxy/util"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

func TestActorOnRequest(t *testing.T) {
	assert.Nil(t, GetActorInfoFromRequest(util.Must(http.NewRequest("GET", "https://example.com", nil))))

	a := jwt.Actor{
		ID: "bobdole",
	}

	r := util.Must(http.NewRequest("GET", "https://example.com", nil))
	r = SetActorInfoOnRequest(r, &a)
	assert.Equal(t, &a, GetActorInfoFromRequest(r))
}
