package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
	"path/filepath"
	"user-service/internal/delivery/messaging" 
	"user-service/internal/infrastructure" 
	"user-service/internal/repository"
	"user-service/internal/usecase"
	"github.com/joho/godotenv"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Load env variables 
	if envErr := godotenv.Load(filepath.Join("..", "..", ".env")); envErr != nil {
        log.Println("No .env file found or error loading .env")
    }
	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// PostgreSQL connection with optimized pool settings
	pgConfig, err := pgxpool.ParseConfig(os.Getenv("PostgreSQL"))
	if err != nil {
		log.Fatalf("Unable to parse PostgreSQL config: %v", err)
	}

	// Connection pool optimization
	pgConfig.MaxConns = 20
	pgConfig.MinConns = 5
	pgConfig.MaxConnLifetime = time.Hour
	pgConfig.MaxConnIdleTime = 30 * time.Minute
	pgConfig.HealthCheckPeriod = 5 * time.Minute

	pgPool, err := pgxpool.NewWithConfig(ctx, pgConfig)
	if err != nil {
		log.Fatalf("Unable to connect to PostgreSQL: %v", err)
	}
	defer pgPool.Close()

	// Configure Redis client with optimized settings
	redisClient := infrastructure.NewRedisClient()
	defer redisClient.Close()

	// Verify Redis connection
	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Setup service layers
	userRepo := repository.NewUserRepo(pgPool)
	redisRepo := repository.NewRedisRepo(redisClient)
	jwtService := infrastructure.NewJWTService() 
	otpService := infrastructure.NewOTPService()
	otpRateLimiter := infrastructure.NewRateLimiter(15*time.Minute, 5)
	userUsecase := usecase.NewUserUsecase(userRepo, redisRepo, jwtService, otpService, otpRateLimiter)

	// Initialize TCP handler
	tcpHandler := tcp.NewTCPHandler(userUsecase)

	// Start TCP server in a goroutine
	go func() {
		log.Println("Starting TCP server on port 3001")
		if err := tcpHandler.Start(":3001"); err != nil {
			log.Fatalf("TCP server failed: %v", err)
		}
	}()

	// Graceful shutdown handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal
	<-sigCh
	log.Println("Received shutdown signal, initiating graceful shutdown...")

	// Shutdown TCP server
	if err := tcpHandler.Stop(); err != nil {
		log.Printf("Error shutting down TCP server: %v", err)
	}

	log.Println("Service shutdown completed successfully")
}