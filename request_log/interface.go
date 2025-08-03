package request_log

import "net/http"

type Logger interface {
	RoundTrip(req *http.Request) (*http.Response, error)
}
