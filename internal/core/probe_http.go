package core

import (
	"bytes"
	"context"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type probeHttp struct {
	probeBase
}

func (p *probeHttp) Invoke(ctx context.Context) (string, error) {
	return p.recordInvokeOutcome(ctx, func(ctx context.Context) (string, error) {
		if p.cfg.ProxyHttp != nil {
			// Proxy HTTP probe
			proxy, err := p.c.getProxyImpl()
			if err != nil {
				return ProbeOutcomeError, err
			}

			h := p.cfg.ProxyHttp
			req := iface.ProxyRequest{
				Method:   h.Method,
				URL:      h.URL,
				Headers:  h.Headers,
				BodyRaw:  h.BodyRaw,
				BodyJson: h.BodyJson,
			}

			// TODO: translate result
			_, err = proxy.ProxyRequest(ctx, httpf.RequestTypeProbe, &req)
			if err != nil {
				return ProbeOutcomeError, err
			} else {
				return ProbeOutcomeSuccess, nil
			}
		} else {
			req := p.s.httpf.
				ForConnection(p.c).
				ForConnectorVersion(p.cv).
				ForRequestType(httpf.RequestTypeProbe).
				New().
				UseContext(ctx).
				Request()

			h := p.cfg.Http
			req.URL(h.URL)
			req.Method(h.Method)

			for h, v := range h.Headers {
				req.AddHeader(h, v)
			}

			if h.BodyJson != nil {
				req.JSON(h.BodyJson)
			} else if h.BodyRaw != nil {
				req.Body(bytes.NewReader(h.BodyRaw))
			} else {
				req.BodyString(h.Body)
			}

			_, err := req.Do()
			if err != nil {
				return ProbeOutcomeError, err
			} else {
				return ProbeOutcomeSuccess, nil
			}
		}
	})
}

var _ iface.Probe = (*probeHttp)(nil)
