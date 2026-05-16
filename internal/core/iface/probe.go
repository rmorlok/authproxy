package iface

import "context"

type Probe interface {
	GetId() string
	Invoke(ctx context.Context) (string, error)
	IsPeriodic() bool
	GetScheduleString() string

	// EffectiveFailureThreshold returns the probe's configured
	// failure_threshold or the system default when unset. The probe runtime
	// uses this to decide when to flip the connection unhealthy.
	EffectiveFailureThreshold() int

	// EffectiveRecoveryThreshold returns the probe's configured
	// recovery_threshold or the system default when unset.
	EffectiveRecoveryThreshold() int
}
