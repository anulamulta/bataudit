package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/joaovrmoraes/bataudit/internal/audit"
	"github.com/joaovrmoraes/bataudit/internal/queue"
	"gorm.io/datatypes"
)

var (
	requestCount = flag.Int("requests", 100, "Total requests to send")
	concurrency  = flag.Int("concurrency", 10, "Number of concurrent goroutines to send requests")
	interval     = flag.Duration("interval", 100*time.Millisecond, "Interval between request batches (e.g. 100ms, 1s)")
	apiURL       = flag.String("api", "http://localhost:8081/audit", "API endpoint URL")
	mode         = flag.String("mode", "api", "Mode to run in: 'api' (send to API) or 'redis' (direct to Redis)")
	redisAddr    = flag.String("redis", "localhost:6379", "Redis server address (used only in redis mode)")
	redisDB      = flag.Int("redis-db", 0, "Redis logical database 0-15 (used only in redis mode)")
	queueName    = flag.String("queue", queue.DefaultQueueName, "Queue name for sending events (used only in redis mode)")
)

// generateRandomAuditEvent creates a random audit event for testing
func generateRandomAuditEvent() audit.Audit {
	statusCodes := []int{200, 201, 204, 400, 401, 403, 404, 500}
	methods := []audit.HTTPMethod{audit.GET, audit.POST, audit.PUT, audit.DELETE}
	paths := []string{"/api/users", "/api/products", "/api/orders", "/api/auth", "/api/settings"}
	environments := []string{"dev", "staging", "production"}
	services := []string{"api-gateway", "user-service", "product-service"}

	statusCode := statusCodes[rand.Intn(len(statusCodes))]
	method := methods[rand.Intn(len(methods))]
	path := paths[rand.Intn(len(paths))]
	environment := environments[rand.Intn(len(environments))]
	service := services[rand.Intn(len(services))]

	return audit.Audit{
		ID:           uuid.New().String(),
		Method:       method,
		Path:         path,
		StatusCode:   statusCode,
		Timestamp:    time.Now(),
		Identifier:   fmt.Sprintf("user-%d", rand.Intn(100)),
		RequestID:    uuid.New().String(),
		UserAgent:    "LoadTester/1.0",
		IP:           fmt.Sprintf("192.168.%d.%d", rand.Intn(255), rand.Intn(255)),
		RequestBody:  datatypes.JSON([]byte(fmt.Sprintf(`{"test": true, "random": %d}`, rand.Intn(1000)))),
		ResponseTime: rand.Int63n(5000),
		ServiceName:  service,
		Environment:  environment,
	}
}

// sendToAPI sends an audit event to the API via HTTP
func sendToAPI(event audit.Audit, client *http.Client) error {
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshaling event: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", *apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "BatAudit-LoadTester/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned non-success status: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

// sendToRedis sends an audit event directly to Redis
func sendToRedis(ctx context.Context, event audit.Audit, redisQueue *queue.RedisQueue) error {
	return redisQueue.Enqueue(ctx, event)
}

func main() {
	flag.Parse()
	fmt.Printf("Starting load test with %d requests, %d concurrent, interval %v\n",
		*requestCount, *concurrency, *interval)
	fmt.Printf("Mode: %s\n", *mode)

	var redisQueue *queue.RedisQueue
	var queueLen int64

	// Setup based on mode
	if *mode == "redis" {
		// Connect to Redis
		var err error
		redisQueue, err = queue.NewRedisQueue(*redisAddr, *redisDB, *queueName)
		if err != nil {
			fmt.Printf("Error connecting to Redis: %v\n", err)
			return
		}
		defer redisQueue.Close()

		// Verify connection
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		queueLen, err = redisQueue.QueueLength(ctx)
		cancel()

		if err != nil {
			fmt.Printf("Error checking queue: %v\n", err)
			return
		}

		fmt.Printf("Connected to Redis. Current queue length: %d\n", queueLen)
	} else if *mode == "api" {
		fmt.Printf("Targeting API at: %s\n", *apiURL)

		// Test API connection
		client := &http.Client{Timeout: 5 * time.Second}
		req, err := http.NewRequestWithContext(context.Background(), "GET", *apiURL, nil)
		if err != nil {
			fmt.Printf("Warning: Could not create test request: %v\n", err)
		} else {
			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("Warning: API connection test failed: %v\n", err)
				fmt.Printf("Will still attempt to send requests...\n")
			} else {
				resp.Body.Close()
				fmt.Printf("API connection successful (status: %d)\n", resp.StatusCode)
			}
		}
	} else {
		fmt.Printf("Error: Invalid mode '%s'. Use 'api' or 'redis'.\n", *mode)
		return
	}

	// Setup HTTP client with appropriate timeouts for API mode
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	// Concurrency control
	semaphore := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	startTime := time.Now()
	successCount := 0
	errorCount := 0
	var mu sync.Mutex

	// Metrics collection
	type responseMetrics struct {
		min     time.Duration
		max     time.Duration
		total   time.Duration
		count   int
		success int
		errors  int
	}

	metrics := responseMetrics{
		min: time.Hour, // Start with a large value to be reduced
	}

	// Real time counters
	ticker := time.NewTicker(1 * time.Second)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				elapsed := time.Since(startTime).Seconds()
				ratePerSecond := float64(successCount+errorCount) / elapsed
				fmt.Printf("[%ds] Sent: %d | Errors: %d | Rate: %.2f req/s\n",
					int(elapsed), successCount, errorCount, ratePerSecond)
				mu.Unlock()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	// Send Requests
	for i := 0; i < *requestCount; i++ {
		wg.Add(1)
		semaphore <- struct{}{} // Add a slot

		go func(reqNum int) {
			defer func() {
				<-semaphore // Release the slot when done
				wg.Done()
			}()

			// Generate random event
			event := generateRandomAuditEvent()

			var err error
			requestStart := time.Now()

			// Send based on mode
			if *mode == "redis" {
				// Send to Redis
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				err = sendToRedis(ctx, event, redisQueue)
				cancel()
			} else {
				// Send to API
				err = sendToAPI(event, client)
			}

			responseTime := time.Since(requestStart)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				fmt.Printf("Error sending event #%d: %v\n", reqNum, err)
				errorCount++
				metrics.errors++
			} else {
				successCount++
				metrics.success++
			}

			// Update metrics
			metrics.count++
			metrics.total += responseTime
			if responseTime < metrics.min {
				metrics.min = responseTime
			}
			if responseTime > metrics.max {
				metrics.max = responseTime
			}
		}(i)

		// Respect interval between batches
		if i > 0 && i%*concurrency == 0 && *interval > 0 {
			time.Sleep(*interval)
		}
	}

	wg.Wait()
	done <- true

	elapsedTime := time.Since(startTime)

	// Calculate average response time
	avgResponseTime := metrics.total / time.Duration(metrics.count)
	if metrics.count == 0 {
		avgResponseTime = 0
	}

	fmt.Printf("\n--- Test Summary ---\n")
	fmt.Printf("Total requests: %d\n", *requestCount)
	fmt.Printf("Successful requests: %d\n", successCount)
	fmt.Printf("Errors: %d\n", errorCount)
	fmt.Printf("Total time: %.2fs\n", elapsedTime.Seconds())
	fmt.Printf("Average rate: %.2f req/s\n", float64(*requestCount)/elapsedTime.Seconds())
	fmt.Printf("Response time (min/avg/max): %v/%v/%v\n",
		metrics.min, avgResponseTime, metrics.max)

	// Check final queue length for Redis mode
	if *mode == "redis" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		finalQueueLen, err := redisQueue.QueueLength(ctx)
		cancel()

		if err == nil {
			fmt.Printf("Final queue length: %d\n", finalQueueLen)
		}
	}
}
