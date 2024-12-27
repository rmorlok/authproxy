package jwt

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"net/http"
)

type Signer interface {
	SignAuthHeader(req *http.Request)
	SignRestyRequest(req *resty.Request) *resty.Request
}

type signer struct {
	token string
}

func (s *signer) SignAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.token))
}

func (s *signer) SignRestyRequest(req *resty.Request) *resty.Request {
	return req.SetAuthToken(s.token)
}

func NewSigner(token string) Signer {
	return &signer{token}
}
