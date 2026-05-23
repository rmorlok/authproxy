package core

import (
	"context"
	"fmt"
	"time"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/aplog"
)

// DefaultProbeNowThrottleWindow caps how often the proxy's 401/403 detection
// path can re-enqueue a probe-now task for the same (connection, probe). A
// 60-second window is plenty for the typical incident shape — a single
// rotation event triggers one probe-now per probe, the probe records its
// outcome, and any further 401s are absorbed without piling up duplicate
// tasks. Shorter windows risk task storms on misbehaving upstreams; longer
// windows defeat the "detection lag → ~immediate" point of the feature.
const DefaultProbeNowThrottleWindow = 60 * time.Second

// probeNowThrottleKey returns the Redis key used by the SETNX-style throttle.
// One key per (connection, probe) so probes on the same connection don't
// throttle each other, and connections under load against the same connector
// don't throttle each other either.
func probeNowThrottleKey(connectionId apid.ID, probeId string) string {
	return fmt.Sprintf("probe_now:throttle:%s:%s", connectionId.String(), probeId)
}

// EnqueueProbeNow enqueues an asynq probe task for every probe on the
// connection, subject to per-(connection, probe) throttling. See the
// iface.C.EnqueueProbeNow contract for the broader contract.
//
// Best-effort by design: any error from looking up the connection or
// enqueueing is logged at warn-level and the function continues. The caller
// is the proxy's 401/403 detection path, which has already sent the
// upstream's response to the customer — failing to enqueue a probe-now is
// not a customer-facing failure (the next scheduled probe will still detect
// the credential problem; we just lose the latency win for this incident).
func (s *service) EnqueueProbeNow(ctx context.Context, connectionId apid.ID) error {
	logger := aplog.NewBuilder(s.logger).
		WithCtx(ctx).
		WithConnectionId(connectionId).
		Build()

	conn, err := s.getConnection(ctx, connectionId)
	if err != nil {
		logger.Warn("probe-now: failed to load connection", "error", err)
		return err
	}

	probes := conn.GetProbes()
	if len(probes) == 0 {
		return nil
	}

	for _, probe := range probes {
		key := probeNowThrottleKey(connectionId, probe.GetId())
		ok, err := s.r.SetNX(ctx, key, "1", DefaultProbeNowThrottleWindow).Result()
		if err != nil {
			logger.Warn("probe-now: throttle check failed",
				"probe_id", probe.GetId(),
				"error", err,
			)
			continue
		}
		if !ok {
			// Already enqueued within the throttle window — skip silently.
			// Logging would be too noisy under sustained 401 traffic.
			continue
		}

		task, err := newProbeTask(connectionId, probe.GetId())
		if err != nil {
			logger.Warn("probe-now: failed to build probe task",
				"probe_id", probe.GetId(),
				"error", err,
			)
			continue
		}
		if _, err := s.ac.EnqueueContext(ctx, task); err != nil {
			logger.Warn("probe-now: failed to enqueue probe task",
				"probe_id", probe.GetId(),
				"error", err,
			)
			continue
		}
		logger.Info("probe-now: probe task enqueued", "probe_id", probe.GetId())
	}
	return nil
}
