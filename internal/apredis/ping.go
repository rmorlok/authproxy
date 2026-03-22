package apredis

import (
	"context"
	"fmt"
	"log"
)

func Ping(ctx context.Context, c Client) bool {
	if c == nil {
		log.Println("redis client is unexpectedly nil ")
		return false
	}

	_, err := c.Ping(ctx).Result()
	if err != nil {
		log.Println(fmt.Errorf("failed to connect to redis server: %w", err))
		return false
	}

	return true
}
