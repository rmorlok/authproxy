package connectors

import "context"

type operation func(ctx context.Context) error
