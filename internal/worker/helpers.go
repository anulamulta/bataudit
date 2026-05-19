package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joaovrmoraes/bataudit/internal/queue"
)

// ConnectToRedisWithRetry tries to connect to Redis with exponential backoff.
// `db` is the Redis logical database (0-15); pass 0 for default behaviour.
func ConnectToRedisWithRetry(address string, db int, queueName string, maxRetries int) (*queue.RedisQueue, error) {
	var redisQueue *queue.RedisQueue
	var err error

	for i := 0; i < maxRetries; i++ {
		slog.Info("Connecting to Redis", "attempt", i+1, "max_retries", maxRetries)
		redisQueue, err = queue.NewRedisQueue(address, db, queueName)
		if err == nil {
			slog.Info("Redis connection established")
			return redisQueue, nil
		}

		slog.Warn("Redis connection failed", "error", err)
		if i < maxRetries-1 {
			waitTime := time.Duration(2<<uint(i)) * time.Second
			slog.Info("Retrying", "delay", waitTime.String())
			time.Sleep(waitTime)
		}
	}

	return nil, fmt.Errorf("could not connect to Redis after %d attempts: %w", maxRetries, err)
}

// SetupSignalHandler sets up a handler for interrupt signals
func SetupSignalHandler(ctx context.Context, cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			slog.Info("Interrupt signal received, shutting down")
			cancel()
		case <-ctx.Done():
			return
		}
	}()
}
