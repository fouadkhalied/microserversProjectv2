package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	//"user-service/internal/config"
	"time"
	"user-service/internal/delivery/messaging"
	"user-service/internal/delivery/nats"
	"user-service/internal/infastructure"
	"user-service/internal/repository"
	"user-service/internal/usecase"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Load configuration
	//cfg := config.Load()

	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// PostgreSQL connection with optimized pool settings
	pgConfig, err := pgxpool.ParseConfig("postgresql://postgres:pfYtJzUVVcksnbRPNwoMUMeAbluqMqgJ@centerbeam.proxy.rlwy.net:44785/railway")
	if err != nil {
		log.Fatalf("Unable to parse PostgreSQL config: %v", err)
	}

	// Configure optimal connection pool
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

	// Configure Redis with connection pooling
	redisClient := redis.NewClient(&redis.Options{
		Addr:         "localhost:6379",
		PoolSize:     10, // Set pool size
		MinIdleConns: 5,  // Keep minimum connections open
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
	defer redisClient.Close()

	// NATS connection
	natsConn, err := nats.ConnectNats()
	if err != nil {
		log.Fatalf("❌ Failed to connect to NATS: %v", err)
	}

	// Setup services
	userRepo := repository.NewUserRepo(pgPool)
	redisRepo := repository.NewRedisRepo(redisClient)
	jwtService := infastructure.NewJWTService()
	userUsecase := usecase.NewUserUsecase(userRepo, redisRepo, jwtService)

	// WebSocket handler
	wsHandler := messaging.NewHandler(userUsecase)

	// Router with WebSocket only
	r := mux.NewRouter()
	r.HandleFunc("/ws", wsHandler.ServeHTTP) // WebSocket endpoint for all user operations
	

	//HTTP Server
	server := &http.Server{
		Addr:    ":3001",
		Handler: r,
	}

	// Graceful shutdown handling
	go func() {
		log.Println("User Service WebSocket server listening on ws://localhost:3001/ws")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Error starting server: %v", err)
		}
	}()

	// Wait for termination signal (SIGINT, SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive a signal
	<-sigCh
	log.Println("Received shutdown signal, shutting down...")

	// Gracefully shutdown the HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}

	// Closing connections gracefully
	natsConn.Close()
	log.Println("Closed NATS connection")

	redisClient.Close()
	log.Println("Closed Redis connection")

	pgPool.Close()
	log.Println("Closed PostgreSQL connection")

	log.Println("Service shutdown complete")
}
