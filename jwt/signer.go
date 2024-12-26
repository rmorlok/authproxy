package jwt

import (
	"fmt"
	"net/http"
)

type Signer interface {
}

type signer struct {
	token string
}

func (s *signer) SignAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.token))
}

func NewSigner(token string) Signer {
	return &signer{token}
}
