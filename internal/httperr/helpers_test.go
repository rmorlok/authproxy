package httperr

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContains(t *testing.T) {
	require.False(t, Contains(nil, "test"))
	require.False(t, Contains(errors.New("test"), "test"))
	require.False(t, Contains(&Error{}, "test"))
	require.True(t, Contains(&Error{ResponseMsg: "test"}, "test"))
	require.True(t, Contains(&Error{ResponseMsg: "this is a test method"}, "test"))
	require.True(t, Contains(&Error{InternalErr: errors.New("test")}, "test"))
	require.True(t, Contains(&Error{InternalErr: errors.New("this is a test method")}, "test"))
}

func TestIsStatus(t *testing.T) {
	require.False(t, IsStatus(nil, http.StatusBadRequest))
	require.False(t, IsStatus(errors.New("test"), http.StatusBadRequest))
	require.False(t, IsStatus(&Error{}, http.StatusBadRequest))
	require.False(t, IsStatus(&Error{Status: http.StatusOK}, http.StatusBadRequest))
	require.True(t, IsStatus(&Error{Status: http.StatusBadRequest}, http.StatusBadRequest))
}

func TestAs(t *testing.T) {
	require.Nil(t, As(nil))
	require.Nil(t, As(errors.New("plain")))

	he := &Error{Status: http.StatusBadRequest, ResponseMsg: "bad"}
	require.Equal(t, he, As(he))
}
