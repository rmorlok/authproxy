package core

import "context"

type operation func(ctx context.Context) error
