package worker

import (
	"time"

	"github.com/joaovrmoraes/bataudit/internal/queue"
)

// Config contains all configurations for the worker
type Config struct {
	// Basic worker configuration
	InitialWorkerCount int
	MinWorkerCount     int
	MaxWorkerCount     int
	MaxRetries         int
	PollDuration       time.Duration

	// Autoscaling configuration
	EnableAutoscaling  bool
	ScaleUpThreshold   int64   // Queue size threshold to scale up
	ScaleDownThreshold int64   // Queue size threshold to scale down
	WorkerScaleFactor  float64 // How aggressively to scale (e.g., 1.5 = increase by 50%)
	CooldownPeriod     time.Duration

	// Redis configuration
	RedisAddress string
	RedisDB      int
	QueueName    string
}

// DefaultConfig returns a default configuration for the worker
func DefaultConfig() *Config {
	return &Config{
		// Basic worker configuration
		InitialWorkerCount: 2,  // Start with fewer workers
		MinWorkerCount:     2,  // Keep at least 2 workers always active
		MaxWorkerCount:     10, // Limit to 10 workers
		MaxRetries:         3,
		PollDuration:       1 * time.Second, // More frequent polling

		// Autoscaling configuration
		EnableAutoscaling:  true,
		ScaleUpThreshold:   15,               // Scale up when there are more than 15 items in the queue (was 20)
		ScaleDownThreshold: 5,                // Scale down when there are fewer than 5 items in the queue
		WorkerScaleFactor:  2.0,              // Scale more aggressively (was 1.5)
		CooldownPeriod:     15 * time.Second, // Reduced from 30 to 15 seconds for faster response

		// Redis configuration
		RedisAddress: "localhost:6379",
		RedisDB:      0,
		QueueName:    queue.DefaultQueueName,
	}
}
