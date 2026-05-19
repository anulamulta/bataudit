# Worker

Background service that consumes audit events from the Redis queue and persists them to PostgreSQL. Supports dynamic autoscaling based on queue depth.

## Responsibility

The Worker is the **bridge between the queue and the database**. It runs continuously, polling Redis for events and writing them to PostgreSQL. It never exposes an HTTP port.

```
Redis queue → Worker → PostgreSQL
```

## How it works

On startup, the Worker launches an initial pool of goroutines (workers). Each worker independently polls the Redis queue at a fixed interval. A separate monitor goroutine runs every 5 seconds and adjusts the number of active workers based on queue depth.

```
main()
  ├── monitor goroutine (every 5s)   → evaluates queue size → scales workers up/down
  └── worker goroutines (N)          → dequeue → deserialize → write to DB (with retry)
```

### Processing loop (per worker)

1. Poll Redis with a 1-second timeout
2. If an event is available, deserialize it from JSON
3. Call `audit.Service.CreateAudit()` to persist it
4. On failure, retry up to `MAX_RETRIES` times with a 2-second wait between attempts
5. If all retries fail, log and discard the event

### Autoscaling logic

The monitor evaluates the queue every 5 seconds. Scaling only happens after the cooldown period has elapsed (default: 15s), except in emergency conditions.

| Condition | Action |
|---|---|
| Queue > `SCALE_UP_THRESHOLD` | Scale up by `WORKER_SCALE_FACTOR` |
| Queue > `SCALE_UP_THRESHOLD × 3` | Aggressive scale up (factor × 1.5) |
| Queue > `SCALE_UP_THRESHOLD × 5` | Scale to maximum immediately (ignores cooldown) |
| Queue < `SCALE_DOWN_THRESHOLD` | Scale down by `WORKER_SCALE_FACTOR` |

Worker count is always clamped between `WORKER_MIN_COUNT` and `WORKER_MAX_COUNT`.

## Default configuration

| Parameter | Default | Description |
|---|---|---|
| Initial workers | `2` | Workers started at launch |
| Min workers | `2` | Floor — never goes below this |
| Max workers | `10` | Ceiling — never exceeds this |
| Max retries | `3` | Attempts per event before discard |
| Poll interval | `1s` | How often each worker checks the queue |
| Scale up threshold | `15` | Queue items that trigger scale up |
| Scale down threshold | `5` | Queue items that trigger scale down |
| Scale factor | `2.0` | Multiplier applied to current worker count when scaling |
| Cooldown period | `15s` | Minimum time between consecutive scaling decisions |

## Environment variables

| Variable | Alt name | Default | Description |
|---|---|---|---|
| `WORKER_INITIAL_COUNT` | `BATAUDIT_INITIAL_WORKER_COUNT` | `2` | Initial worker count |
| `WORKER_MIN_COUNT` | `BATAUDIT_MIN_WORKER_COUNT` | `2` | Minimum worker count |
| `WORKER_MAX_COUNT` | `BATAUDIT_MAX_WORKER_COUNT` | `10` | Maximum worker count |
| `WORKER_MAX_RETRIES` | `BATAUDIT_MAX_RETRIES` | `3` | Retries per event |
| `WORKER_POLL_DURATION` | `BATAUDIT_POLL_DURATION` | `1s` | Poll interval (e.g. `500ms`, `2s`) |
| `ENABLE_AUTOSCALING` | `BATAUDIT_ENABLE_AUTOSCALING` | `true` | Enable/disable autoscaling |
| `SCALE_UP_THRESHOLD` | `BATAUDIT_SCALE_UP_THRESHOLD` | `15` | Queue depth to trigger scale up |
| `SCALE_DOWN_THRESHOLD` | `BATAUDIT_SCALE_DOWN_THRESHOLD` | `5` | Queue depth to trigger scale down |
| `WORKER_SCALE_FACTOR` | `BATAUDIT_WORKER_SCALE_FACTOR` | `2.0` | Scaling multiplier |
| `COOLDOWN_PERIOD` | `BATAUDIT_COOLDOWN_PERIOD` | `15s` | Cooldown between scale decisions |
| `REDIS_ADDRESS` | `BATAUDIT_REDIS_ADDRESS` | `localhost:6379` | Redis address |
| `REDIS_DB` | `BATAUDIT_REDIS_DB` | `0` | Redis logical database (0-15) |
| `QUEUE_NAME` | `BATAUDIT_QUEUE_NAME` | `bataudit:events` | Redis queue key |
| `DB_HOST` | — | `localhost` | Database host |
| `DB_PORT` | — | `5432` | Database port |
| `DB_USER` | — | — | Database user |
| `DB_PASSWORD` | — | — | Database password |
| `DB_NAME` | — | — | Database name |

> Each variable accepts two names for flexibility: a short form and a `BATAUDIT_`-prefixed form. The short form takes precedence.

## Graceful shutdown

The Worker listens for `SIGINT` and `SIGTERM`. On signal:
1. The context is cancelled
2. Each worker goroutine finishes its current event (or poll cycle) and exits
3. The monitor goroutine exits
4. The process terminates cleanly after all goroutines finish

## Dependencies

- **Redis** — source queue (`bataudit:events`)
- **PostgreSQL** — destination for persisted events

## Running locally

```bash
# Start dependencies
docker compose up -d postgres redis

# Run with defaults
DB_HOST=localhost DB_USER=batuser DB_PASSWORD=batpassword DB_NAME=batdb \
  go run ./cmd/api/worker

# Run with custom scaling
DB_HOST=localhost DB_USER=batuser DB_PASSWORD=batpassword DB_NAME=batdb \
  WORKER_INITIAL_COUNT=1 ENABLE_AUTOSCALING=false \
  go run ./cmd/api/worker
```
