package apredis

import (
	"context"
	"log"

	"github.com/pkg/errors"
)

func Ping(ctx context.Context, c Client) bool {
	if c == nil {
		log.Println("redis client is unexpectedly nil ")
		return false
	}

	_, err := c.Ping(ctx).Result()
	if err != nil {
		log.Println(errors.Wrap(err, "failed to connect to redis server"))
		return false
	}

	return true
}
