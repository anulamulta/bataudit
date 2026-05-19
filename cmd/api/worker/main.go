package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joaovrmoraes/bataudit/internal/anomaly"
	"github.com/joaovrmoraes/bataudit/internal/audit"
	"github.com/joaovrmoraes/bataudit/internal/config"
	"github.com/joaovrmoraes/bataudit/internal/db"
	"github.com/joaovrmoraes/bataudit/internal/healthcheck"
	"github.com/joaovrmoraes/bataudit/internal/notification"
	"github.com/joaovrmoraes/bataudit/internal/tiering"
	"github.com/joaovrmoraes/bataudit/internal/worker"
	"gorm.io/datatypes"
)

func main() {
	setupLogger()

	conn, err := db.Init()
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	sqlDB, _ := conn.DB()
	defer sqlDB.Close()

	cfg := worker.DefaultConfig()
	worker.ConfigureFromEnv(cfg)

	maxRetries := 5

	redisQueue, err := worker.ConnectToRedisWithRetry(
		cfg.RedisAddress,
		cfg.RedisDB,
		cfg.QueueName,
		maxRetries,
	)
	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisQueue.Close()

	repository := audit.NewRepository(conn)
	auditService := audit.NewService(repository)

	// Build notification sender.
	notifRepo := notification.NewRepository(conn)
	notifSender := notification.NewSender(
		notifRepo,
		config.GetEnv("VAPID_PUBLIC_KEY", ""),
		config.GetEnv("VAPID_PRIVATE_KEY", ""),
		config.GetEnv("VAPID_SUBJECT", ""),
	)

	// Build alert sink: persists system.alert events and sends notifications.
	sink := &auditAlertSink{svc: auditService, notif: notifSender}

	anomalyRepo := anomaly.NewRepository(conn)
	detector := anomaly.NewDetector(anomalyRepo, sink)

	workerService := worker.NewService(cfg, auditService, redisQueue).WithDetector(detector)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker.SetupSignalHandler(ctx, cancel)

	detector.Start(ctx) // background goroutine for silent-service checks

	// Start healthcheck poller.
	hcRepo := healthcheck.NewRepository(conn)
	hcSink := &healthEventSink{svc: auditService, notif: notifSender}
	hcPoller := healthcheck.NewPoller(hcRepo, hcSink)
	hcPoller.Start(ctx)

	// Start data tiering scheduler (aggregates old events nightly).
	tieringRepo := tiering.NewRepository(conn)
	tieringScheduler := tiering.NewSchedulerFromEnv(tieringRepo, config.GetEnv)
	go tieringScheduler.Start(ctx)

	slog.Info("Starting BatAudit worker service", "autoscaling", cfg.EnableAutoscaling)
	if err := workerService.Start(ctx); err != nil {
		slog.Error("Worker service failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Application shut down successfully")
}

// auditAlertSink implements anomaly.AlertSink by writing system.alert events
// and dispatching notifications.
type auditAlertSink struct {
	svc   *audit.Service
	notif *notification.Sender
}

func (s *auditAlertSink) CreateAlert(
	projectID, serviceName, environment string,
	ruleType anomaly.RuleType,
	details map[string]any,
) error {
	payload, _ := json.Marshal(details)

	env := environment
	if env == "" || env == "unknown" {
		env = "production"
	}

	event := audit.Audit{
		ID:          uuid.New().String(),
		EventType:   "system.alert",
		Path:        string(ruleType),
		Identifier:  "system",
		ServiceName: serviceName,
		Environment: env,
		ProjectID:   projectID,
		Timestamp:   time.Now(),
		RequestBody: datatypes.JSON(payload),
	}
	if err := s.svc.CreateAudit(event); err != nil {
		return err
	}

	// Fire-and-forget: send push/webhook notifications.
	go s.notif.NotifyAll(context.Background(), notification.AlertPayload{
		EventID:     event.ID,
		ProjectID:   projectID,
		ServiceName: serviceName,
		RuleType:    string(ruleType),
		Timestamp:   event.Timestamp,
		Details:     details,
	})

	return nil
}

// healthEventSink implements healthcheck.EventSink by writing system.healthcheck.* events
// and dispatching notifications.
type healthEventSink struct {
	svc   *audit.Service
	notif *notification.Sender
}

func (s *healthEventSink) CreateHealthEvent(projectID, monitorName, monitorURL, eventType string, details map[string]any) error {
	payload, _ := json.Marshal(details)

	event := audit.Audit{
		ID:          uuid.New().String(),
		EventType:   eventType,
		Path:        monitorURL,
		Identifier:  "system",
		ServiceName: monitorName,
		Environment: "production",
		ProjectID:   projectID,
		Timestamp:   time.Now(),
		RequestBody: datatypes.JSON(payload),
	}
	if err := s.svc.CreateAudit(event); err != nil {
		return err
	}

	go s.notif.NotifyAll(context.Background(), notification.AlertPayload{
		EventID:     event.ID,
		ProjectID:   projectID,
		ServiceName: monitorName,
		RuleType:    eventType,
		Timestamp:   event.Timestamp,
		Details:     details,
	})

	return nil
}

func setupLogger() {
	level := slog.LevelInfo
	switch config.GetEnv("LOG_LEVEL", "info") {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}
