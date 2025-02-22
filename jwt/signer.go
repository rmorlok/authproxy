package jwt

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"net/http"
	"net/url"
)

type Signer interface {
	SignUrlQuery(url string) string
	SignAuthHeader(req *http.Request)
	SignRestyRequest(req *resty.Request) *resty.Request
}

type signer struct {
	token string
}

func (s *signer) SignUrlQuery(urlVal string) string {
	parsedUrl, err := url.Parse(urlVal)
	if err != nil {
		return urlVal + "?jwt=" + s.token
	}

	query := parsedUrl.Query()
	query.Set("jwt", s.token)
	parsedUrl.RawQuery = query.Encode()

	return parsedUrl.String()
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
