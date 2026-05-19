package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	DefaultQueueName = "bataudit:events"
)

type RedisQueue struct {
	client *redis.Client
	queue  string
}

// NewRedisQueue - creates a new RedisQueue instance.
// `db` is the Redis logical database (0-15). Useful when sharing a Redis
// instance across services that need isolated keyspaces (e.g. BatAudit on
// db 3 while other apps use db 0-2).
func NewRedisQueue(addr string, db int, queue string) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, err
	}

	return &RedisQueue{
		client: client,
		queue:  queue,
	}, nil
}

// Enqueue - add a new item to the queue
func (q *RedisQueue) Enqueue(ctx context.Context, item interface{}) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}

	return q.client.RPush(ctx, q.queue, data).Err()
}

// Dequeue - remove and return an item from the queue, optionally using non-blocking check
func (q *RedisQueue) Dequeue(ctx context.Context) ([]byte, error) {
	timeout := 1 * time.Second

	result, err := q.client.BLPop(ctx, timeout, q.queue).Result()

	if err == redis.Nil {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if len(result) < 2 {
		return nil, nil
	}

	return []byte(result[1]), nil
}

// QueueLength - returns the current length of the queue
func (q *RedisQueue) QueueLength(ctx context.Context) (int64, error) {
	return q.client.LLen(ctx, q.queue).Result()
}

// Close - close the Redis client connection
func (q *RedisQueue) Close() error {
	return q.client.Close()
}
