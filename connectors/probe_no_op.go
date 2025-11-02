package connectors

import (
	"context"

	"github.com/rmorlok/authproxy/connectors/iface"
)

type probeNoOp struct {
	probeBase
}

func (p *probeNoOp) Invoke(ctx context.Context) (string, error) {
	return p.recordInvokeOutcome(ctx, func(ctx context.Context) (string, error) {
		return ProbeOutcomeSuccess, nil
	})
}

var _ iface.Probe = (*probeNoOp)(nil)
