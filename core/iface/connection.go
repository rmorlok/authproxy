package iface

import (
	"context"
	"net/http"

	"github.com/rmorlok/authproxy/request_log"
)

type Connection interface {
	GetProbe(probeId string) (Probe, error)
	GetProbes() []Probe
	ProxyRequest(
		ctx context.Context,
		reqType request_log.RequestType,
		req *ProxyRequest,
	) (*ProxyResponse, error)
	ProxyRequestRaw(
		ctx context.Context,
		reqType request_log.RequestType,
		req *ProxyRequest,
		w http.ResponseWriter,
	) error
}
