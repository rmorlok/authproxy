package core

import (
	"bytes"
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/core/iface"
	"github.com/rmorlok/authproxy/internal/httpf"
)

type probeHttp struct {
	probeBase
}

// Invoke runs the probe and reports either success or error. Both branches
// (proxy and raw) treat any non-2xx upstream response as a probe failure —
// without this, an upstream that 401s the proxied request would silently be
// considered a success and the probe-driven health signal would never flip
// to unhealthy. The recorded error carries the status code so operators can
// see what the upstream actually returned.
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

			resp, err := proxy.ProxyRequest(ctx, httpf.RequestTypeProbe, &req)
			if err != nil {
				return ProbeOutcomeError, err
			}
			if resp == nil {
				return ProbeOutcomeError, fmt.Errorf("probe upstream returned no response")
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return ProbeOutcomeError, fmt.Errorf("probe upstream returned status %d", resp.StatusCode)
			}
			return ProbeOutcomeSuccess, nil
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

			resp, err := req.Do()
			if err != nil {
				return ProbeOutcomeError, err
			}
			if resp == nil {
				return ProbeOutcomeError, fmt.Errorf("probe upstream returned no response")
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return ProbeOutcomeError, fmt.Errorf("probe upstream returned status %d", resp.StatusCode)
			}
			return ProbeOutcomeSuccess, nil
		}
	})
}

var _ iface.Probe = (*probeHttp)(nil)
