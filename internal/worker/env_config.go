package worker

import (
	"os"
	"strconv"
	"time"
)

// ConfigureFromEnv configures the worker service from environment variables
func ConfigureFromEnv(config *Config) {
	// Basic worker configuration
	if val := os.Getenv("WORKER_INITIAL_COUNT"); val != "" {
		if count, err := strconv.Atoi(val); err == nil && count > 0 {
			config.InitialWorkerCount = count
		}
	} else if val := os.Getenv("BATAUDIT_INITIAL_WORKER_COUNT"); val != "" {
		if count, err := strconv.Atoi(val); err == nil && count > 0 {
			config.InitialWorkerCount = count
		}
	}

	if val := os.Getenv("WORKER_MIN_COUNT"); val != "" {
		if count, err := strconv.Atoi(val); err == nil && count > 0 {
			config.MinWorkerCount = count
		}
	} else if val := os.Getenv("BATAUDIT_MIN_WORKER_COUNT"); val != "" {
		if count, err := strconv.Atoi(val); err == nil && count > 0 {
			config.MinWorkerCount = count
		}
	}

	if val := os.Getenv("WORKER_MAX_COUNT"); val != "" {
		if count, err := strconv.Atoi(val); err == nil && count > 0 {
			config.MaxWorkerCount = count
		}
	} else if val := os.Getenv("BATAUDIT_MAX_WORKER_COUNT"); val != "" {
		if count, err := strconv.Atoi(val); err == nil && count > 0 {
			config.MaxWorkerCount = count
		}
	}

	if val := os.Getenv("WORKER_MAX_RETRIES"); val != "" {
		if retries, err := strconv.Atoi(val); err == nil && retries > 0 {
			config.MaxRetries = retries
		}
	} else if val := os.Getenv("BATAUDIT_MAX_RETRIES"); val != "" {
		if retries, err := strconv.Atoi(val); err == nil && retries > 0 {
			config.MaxRetries = retries
		}
	}

	if val := os.Getenv("WORKER_POLL_DURATION"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil && duration > 0 {
			config.PollDuration = duration
		}
	} else if val := os.Getenv("BATAUDIT_POLL_DURATION"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil && duration > 0 {
			config.PollDuration = duration
		}
	}

	// Autoscaling configuration
	if val := os.Getenv("ENABLE_AUTOSCALING"); val != "" {
		switch val {
		case "true", "1", "yes", "y", "on":
			config.EnableAutoscaling = true
		case "false", "0", "no", "n", "off":
			config.EnableAutoscaling = false
		}
	} else if val := os.Getenv("BATAUDIT_ENABLE_AUTOSCALING"); val != "" {
		switch val {
		case "true", "1", "yes", "y", "on":
			config.EnableAutoscaling = true
		case "false", "0", "no", "n", "off":
			config.EnableAutoscaling = false
		}
	}

	if val := os.Getenv("SCALE_UP_THRESHOLD"); val != "" {
		if threshold, err := strconv.ParseInt(val, 10, 64); err == nil && threshold > 0 {
			config.ScaleUpThreshold = threshold
		}
	} else if val := os.Getenv("BATAUDIT_SCALE_UP_THRESHOLD"); val != "" {
		if threshold, err := strconv.ParseInt(val, 10, 64); err == nil && threshold > 0 {
			config.ScaleUpThreshold = threshold
		}
	}

	if val := os.Getenv("SCALE_DOWN_THRESHOLD"); val != "" {
		if threshold, err := strconv.ParseInt(val, 10, 64); err == nil && threshold > 0 {
			config.ScaleDownThreshold = threshold
		}
	} else if val := os.Getenv("BATAUDIT_SCALE_DOWN_THRESHOLD"); val != "" {
		if threshold, err := strconv.ParseInt(val, 10, 64); err == nil && threshold > 0 {
			config.ScaleDownThreshold = threshold
		}
	}

	if val := os.Getenv("WORKER_SCALE_FACTOR"); val != "" {
		if factor, err := strconv.ParseFloat(val, 64); err == nil && factor > 0 {
			config.WorkerScaleFactor = factor
		}
	} else if val := os.Getenv("BATAUDIT_WORKER_SCALE_FACTOR"); val != "" {
		if factor, err := strconv.ParseFloat(val, 64); err == nil && factor > 0 {
			config.WorkerScaleFactor = factor
		}
	}

	if val := os.Getenv("COOLDOWN_PERIOD"); val != "" {
		if period, err := time.ParseDuration(val); err == nil && period > 0 {
			config.CooldownPeriod = period
		}
	} else if val := os.Getenv("BATAUDIT_COOLDOWN_PERIOD"); val != "" {
		if period, err := time.ParseDuration(val); err == nil && period > 0 {
			config.CooldownPeriod = period
		}
	}

	// Redis configuration
	// Support both specific env var and general one
	if val := os.Getenv("REDIS_ADDRESS"); val != "" {
		config.RedisAddress = val
	} else if val := os.Getenv("BATAUDIT_REDIS_ADDRESS"); val != "" {
		config.RedisAddress = val
	}

	// REDIS_DB lets the worker target a specific Redis logical database (0-15).
	// Default is 0. Useful in multi-tenant Redis setups where other services
	// share the instance.
	if val := os.Getenv("REDIS_DB"); val != "" {
		if db, err := strconv.Atoi(val); err == nil && db >= 0 && db <= 15 {
			config.RedisDB = db
		}
	} else if val := os.Getenv("BATAUDIT_REDIS_DB"); val != "" {
		if db, err := strconv.Atoi(val); err == nil && db >= 0 && db <= 15 {
			config.RedisDB = db
		}
	}

	if val := os.Getenv("QUEUE_NAME"); val != "" {
		config.QueueName = val
	} else if val := os.Getenv("BATAUDIT_QUEUE_NAME"); val != "" {
		config.QueueName = val
	}
}
