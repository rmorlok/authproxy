package iface

import "context"

type Probe interface {
	GetId() string
	Invoke(ctx context.Context) (string, error)
	IsPeriodic() bool
	GetScheduleString() string
}
