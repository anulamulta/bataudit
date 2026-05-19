package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joaovrmoraes/bataudit/internal/anomaly"
	"github.com/joaovrmoraes/bataudit/internal/auth"
	"github.com/joaovrmoraes/bataudit/internal/config"
	"github.com/joaovrmoraes/bataudit/internal/db"
	"github.com/joaovrmoraes/bataudit/internal/queue"
	"gorm.io/gorm"
)

func main() {
	setupLogger()

	conn := connectDB()
	sqlDB, err := conn.DB()
	if err != nil {
		slog.Error("Failed to get underlying DB connection", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	redisQueue := connectRedis()
	defer redisQueue.Close()

	jwtSecret := config.GetEnv("JWT_SECRET", "change-me-in-production")
	authRepo := auth.NewRepository(conn)
	authService := auth.NewService(authRepo, jwtSecret)

	// Seed default anomaly rules whenever a new project is auto-created.
	anomalyRepo := anomaly.NewRepository(conn)
	authService.OnProjectCreated = func(projectID string) {
		if err := anomalyRepo.CreateDefaultRules(projectID); err != nil {
			slog.Error("Failed to seed anomaly rules", "project_id", projectID, "error", err)
		}
	}

	r := gin.Default()
	r.Use(cors.Default())

	registerRoutes(r, conn, authService, redisQueue)

	port := config.GetEnv("API_WRITER_PORT", "8081")
	slog.Info("Writer server running", "port", port)
	if err := r.Run(":" + port); err != nil {
		slog.Error("Writer server failed", "error", err)
	}
}

func connectDB() *gorm.DB {
	const maxRetries = 5
	var conn *gorm.DB
	var err error
	for i := 0; i < maxRetries; i++ {
		slog.Info("Connecting to database", "attempt", i+1, "max_retries", maxRetries)
		conn, err = db.Init()
		if err == nil {
			slog.Info("Database connection established")
			return conn
		}
		slog.Warn("Database connection failed", "error", err)
		if i < maxRetries-1 {
			time.Sleep(5 * time.Second)
		}
	}
	slog.Error("Could not connect to database", "error", err, "attempts", maxRetries)
	os.Exit(1)
	return nil
}

func connectRedis() *queue.RedisQueue {
	const maxRetries = 5
	address := config.GetEnv("REDIS_ADDRESS", "localhost:6379")
	db := config.GetEnvAsInt("REDIS_DB", 0)
	slog.Info("Connecting to Redis", "address", address, "db", db)
	var rq *queue.RedisQueue
	var err error
	for i := 0; i < maxRetries; i++ {
		slog.Info("Connecting to Redis", "attempt", i+1, "max_retries", maxRetries)
		rq, err = queue.NewRedisQueue(address, db, queue.DefaultQueueName)
		if err == nil {
			slog.Info("Redis connection established")
			return rq
		}
		slog.Warn("Redis connection failed", "error", err)
		if i < maxRetries-1 {
			time.Sleep(5 * time.Second)
		}
	}
	slog.Error("Failed to connect to Redis", "error", err, "attempts", maxRetries)
	os.Exit(1)
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
