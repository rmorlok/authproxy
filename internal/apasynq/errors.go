package apasynq

import (
	"errors"

	"github.com/hibiken/asynq"
)

func IsNonRetriable(err error) bool {
	return errors.Is(err, asynq.SkipRetry)
}

func IsRetriable(err error) bool {
	return err != nil && !errors.Is(err, asynq.SkipRetry)
}
