// File: internal/delivery/messaging/handler.go
// Optimized NATS messaging implementation

package nats

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
	"strings"
	"github.com/nats-io/nats.go"

	"user-service/internal/domain"
	"user-service/internal/usecase"
)

var (
	nc         *nats.Conn
	jetStream  nats.JetStreamContext
	once       sync.Once
	natsConfig = &natsConfiguration{
		connectionTimeout: 5 * time.Second,
		reconnectWait:     1 * time.Second,
		maxReconnects:     10,
		handlerTimeout:    2 * time.Second,
		queueGroup:        "user-service",
	}
)

type natsConfiguration struct {
	connectionTimeout time.Duration
	reconnectWait     time.Duration
	maxReconnects     int
	handlerTimeout    time.Duration
	queueGroup        string
}

type Handler struct {
	userUC  *usecase.UserUsecase
	msgPool sync.Pool // Message object pool for reduced allocations
}

// ConnectNats establishes a NATS connection with improved options
func ConnectNats() (*nats.Conn, error) {
	var err error
	
	once.Do(func() {
		if nc != nil && nc.IsConnected() {
			log.Println("✅ NATS already connected.")
			return
		}

		// Configure connection options for better performance and reliability
		opts := []nats.Option{
			nats.Name("user-service"),
			nats.Timeout(natsConfig.connectionTimeout),
			nats.ReconnectWait(natsConfig.reconnectWait),
			nats.MaxReconnects(natsConfig.maxReconnects),
			nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
				log.Printf("NATS disconnected: %v", err)
			}),
			nats.ReconnectHandler(func(nc *nats.Conn) {
				log.Println("NATS reconnected")
			}),
			nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
				log.Printf("NATS error: %v", err)
			}),
			nats.DrainTimeout(10 * time.Second),
		}

		// Connect with options
		nc, err = nats.Connect("http://localhost:4222", opts...)
		if err != nil {
			log.Println("❌ Failed to connect to NATS:", err)
			return
		}

		// Setup JetStream for more robust messaging if available
		jetStream, err = nc.JetStream()
		if err != nil {
			log.Println("⚠️ JetStream not available, falling back to core NATS:", err)
		}

		log.Println("✅ Connected to NATS successfully.")
	})

	return nc, err
}

func NewHandler(nc *nats.Conn, userUC *usecase.UserUsecase) *Handler {
	h := &Handler{
		userUC: userUC,
		msgPool: sync.Pool{
			New: func() interface{} {
				return new(domain.User)
			},
		},
	}

	// Use queue subscriptions for load balancing across multiple instances
	_, err := nc.QueueSubscribe("user.register", natsConfig.queueGroup, h.handleRegister)
	if err != nil {
		log.Println("❌ Failed to subscribe to user.register:", err)
	}

	_, err = nc.QueueSubscribe("user.login", natsConfig.queueGroup, h.handleLogin)
	if err != nil {
		log.Println("❌ Failed to subscribe to user.login:", err)
	}

	// Add health check subject
	_, err = nc.Subscribe("user.health", func(msg *nats.Msg) {
		msg.Respond([]byte(`{"status":"healthy","timestamp":"` + time.Now().String() + `"}`))
	})
	if err != nil {
		log.Println("❌ Failed to subscribe to health check:", err)
	}

	return h
}

func (h *Handler) handleRegister(msg *nats.Msg) {
    // Create context with timeout
    ctx, cancel := context.WithTimeout(context.Background(), natsConfig.handlerTimeout)
    defer cancel()

    // Get user object from pool to reduce allocations
    user := h.msgPool.Get().(*domain.User)
    defer h.msgPool.Put(user)
    
    // Reset fields to avoid data leakage between requests
    *user = domain.User{}

    // Use a more efficient decoder
    decoder := json.NewDecoder(bytes.NewReader(msg.Data))
    decoder.DisallowUnknownFields() // Adds some validation
    
    if err := decoder.Decode(user); err != nil {
        log.Println("Error decoding user:", err)
        msg.Respond([]byte(`{"error":"invalid input"}`))
        return
    }
    
    // Basic validation to fail fast before expensive operations
    if user.Username == "" || user.Password == "" || user.Email == "" {
        msg.Respond([]byte(`{"error":"missing required fields"}`))
        return
    }
    
    // Use a channel to handle timeouts more gracefully
    doneCh := make(chan error, 1)
    
    go func() {
        doneCh <- h.userUC.RegisterUser(ctx, user)
    }()
    
    // Wait for registration to complete or timeout
    select {
    case err := <-doneCh:
        if err != nil {
            log.Println("Register failed:", err)
            if strings.Contains(err.Error(), "username already exists") {
                msg.Respond([]byte(`{"error":"username already exists"}`))
            } else {
                msg.Respond([]byte(`{"error":"registration failed"}`))
            }
            return
        }
        
        // Registration successful
        msg.Respond([]byte(`{"status":"registered","user_id":"` + user.ID + `"}`))
        
    case <-ctx.Done():
        // Timeout occurred
        log.Println("Registration timed out")
        msg.Respond([]byte(`{"error":"registration timed out"}`))
    }
}
func (h *Handler) handleLogin(msg *nats.Msg) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), natsConfig.handlerTimeout)
	defer cancel()

	// Get user object from pool to reduce allocations
	user := h.msgPool.Get().(*domain.User)
	defer h.msgPool.Put(user)
	
	// Reset fields to avoid data leakage between requests
	*user = domain.User{}

	if err := json.Unmarshal(msg.Data, user); err != nil {
		log.Println("Error decoding user:", err)
		msg.Respond([]byte(`{"error":"invalid input"}`))
		return
	}

	token, err := h.userUC.LoginUser(ctx, user.Username, user.Password)
	if err != nil {
		log.Println("Login failed:", err)
		msg.Respond([]byte(`{"error":"login failed"}`))
		return
	}

	// Prepare response with pre-allocated buffer for better performance
	responseMap := map[string]string{"token": token, "timestamp": time.Now().String()}
	response, _ := json.Marshal(responseMap)
	msg.Respond(response)
}

// CloseNats closes the NATS connection gracefully with drain.
func CloseNats() {
	if nc != nil && nc.IsConnected() {
		// Drain ensures all pending messages are processed before closing
		err := nc.Drain()
		if err != nil {
			log.Printf("Error draining NATS connection: %v", err)
			nc.Close()
		}
		log.Println("✅ NATS connection closed gracefully.")
	}
}

// GetStatus returns the current NATS connection status
func GetStatus() string {
	if nc == nil {
		return "not initialized"
	}
	if nc.IsConnected() {
		return "connected"
	}
	return "disconnected"
}
