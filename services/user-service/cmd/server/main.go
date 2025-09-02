package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"user-service-new/internal/application/services"
	"user-service-new/internal/infrastructure"
	postgresRepo "user-service-new/internal/infrastructure/db/postgres"
	"user-service-new/internal/interface/tcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Load environment variables from project root
	if err := godotenv.Load("../../.env"); err != nil {
		log.Printf("No .env file found in project root: %v", err)
		// Try current directory
		if err := godotenv.Load(".env"); err != nil {
			log.Printf("No .env file found in current directory: %v", err)
		}
	}

	// Initialize database
	db, err := initDatabase()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Printf("Connected to database: %v", db)

	// // Auto migrate database
	// if err := db.AutoMigrate(&postgresRepo.UserModel{}); err != nil {
	// 	log.Fatalf("Failed to migrate database: %v", err)
	// }

	// Initialize infrastructure services
	redisService := infrastructure.NewRedisService()
	defer redisService.Close()

	jwtService := infrastructure.NewJWTService()
	otpService := infrastructure.NewOTPService()
	rateLimiter := infrastructure.NewRateLimiter(15*time.Minute, 5)

	// Initialize repositories
	userRepo := postgresRepo.NewUserRepository(db)
	idempotencyRepo := postgresRepo.NewIdempotencyRepository(db)

	// Initialize services
	userService := services.NewUserService(
		userRepo,
		idempotencyRepo,
		redisService,
		jwtService,
		otpService,
		rateLimiter,
	)

	// Initialize TCP handler
	tcpHandler := tcp.NewTCPHandler(userService)

	// Start TCP server in a goroutine
	go func() {
		port := os.Getenv("TCP_PORT")
		if port == "" {
			port = "3001"
		}

		log.Printf("Starting TCP server on port %s", port)
		if err := tcpHandler.Start(":" + port); err != nil {
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

func initDatabase() (*gorm.DB, error) {
	// Check for DATABASE_URL first
	dsn := os.Getenv("DATABASE_URL")
	log.Printf("DATABASE_URL from environment: %s", dsn)

	

	log.Printf("Connecting to database with DSN: %s", dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Configure connection pool from environment variables
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// Get connection pool settings from environment
	maxIdleConns := infrastructure.GetEnvAsInt("DB_MAX_IDLE_CONNS", 10)
	maxOpenConns := infrastructure.GetEnvAsInt("DB_MAX_OPEN_CONNS", 100)
	connMaxLifetime := infrastructure.GetEnvAsDuration("DB_CONN_MAX_LIFETIME", time.Hour)

	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	return db, nil
}
