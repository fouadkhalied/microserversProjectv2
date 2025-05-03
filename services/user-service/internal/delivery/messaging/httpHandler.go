// internal/delivery/messaging/handler.go

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
	"user-service/internal/domain"
	"user-service/internal/usecase"

	"github.com/gorilla/mux"
	"golang.org/x/time/rate"
)

const (
	// Performance settings
	maxConcurrentRequests = 10000
	handlerTimeout        = 5 * time.Second
	rateLimitRequests     = 5000 // Requests per second
	rateLimitBurst        = 1000 // Burst capacity
)

// Handler manages HTTP JSON message processing
type Handler struct {
	userUC         *usecase.UserUsecase
	msgPool        sync.Pool // Message object pool for reduced allocations
	activeRequests int32     // Atomic counter for active requests
	limiter        *rate.Limiter
	metrics        *Metrics
}

// Metrics tracks performance data
type Metrics struct {
	totalRequests      uint64
	successfulRequests uint64
	failedRequests     uint64
	totalLatency       time.Duration
	mutex              sync.RWMutex
	startTime          time.Time
}

// Response represents a standard API response format
type Response struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Code    int         `json:"code,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// NewHandler creates a new HTTP JSON message handler
func NewHandler(userUC *usecase.UserUsecase) *Handler {
	return &Handler{
		userUC: userUC,
		msgPool: sync.Pool{
			New: func() interface{} {
				return new(domain.User)
			},
		},
		limiter: rate.NewLimiter(rate.Limit(rateLimitRequests), rateLimitBurst),
		metrics: &Metrics{
			startTime: time.Now(),
		},
	}
}

// GetMetrics returns current metrics
func (h *Handler) GetMetrics() map[string]interface{} {
	h.metrics.mutex.RLock()
	defer h.metrics.mutex.RUnlock()
	
	uptime := time.Since(h.metrics.startTime)
	var avgLatency time.Duration
	if h.metrics.successfulRequests > 0 {
		avgLatency = time.Duration(int64(h.metrics.totalLatency) / int64(h.metrics.successfulRequests))
	}
	
	return map[string]interface{}{
		"totalRequests":      h.metrics.totalRequests,
		"successfulRequests": h.metrics.successfulRequests,
		"failedRequests":     h.metrics.failedRequests,
		"avgLatencyMs":       avgLatency.Milliseconds(),
		"activeRequests":     atomic.LoadInt32(&h.activeRequests),
		"uptimeSeconds":      uptime.Seconds(),
		"requestsPerSecond":  float64(h.metrics.totalRequests) / uptime.Seconds(),
	}
}

// ServeHTTP handles HTTP requests with JSON data
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Apply rate limiting
	if !h.limiter.Allow() {
		h.sendJSONError(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// Increment active request counter
	if atomic.AddInt32(&h.activeRequests, 1) > maxConcurrentRequests {
		atomic.AddInt32(&h.activeRequests, -1) // Decrement counter
		h.sendJSONError(w, "Server overloaded", http.StatusServiceUnavailable)
		return
	}
	defer atomic.AddInt32(&h.activeRequests, -1)
	
	// Update metrics
	atomic.AddUint64(&h.metrics.totalRequests, 1)
	
	// Track request timing
	startTime := time.Now()
	
	// Set request timeout
	ctx, cancel := context.WithTimeout(r.Context(), handlerTimeout)
	defer cancel()
	
	// Get method from URL path
	method := mux.Vars(r)["method"]
	if method == "" {
		h.sendJSONError(w, "Method not specified", http.StatusBadRequest)
		atomic.AddUint64(&h.metrics.failedRequests, 1)
		return
	}

	// Extract request ID from header for tracking
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = "unknown" // Fallback if client doesn't provide ID
	}

	// Read the JSON request body
	log.Printf("Reading JSON request for method: %s, requestID: %s", method, requestID)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading request body: %v", err)
		h.sendJSONError(w, fmt.Sprintf("Error reading request: %v", err), http.StatusBadRequest)
		atomic.AddUint64(&h.metrics.failedRequests, 1)
		return
	}
	defer r.Body.Close()

	// Process the message
	result, err := h.handleJSONMessage(ctx, body, method)
	if err != nil {
		log.Printf("Error handling message: %v", err)
		h.sendJSONError(w, err.Error(), http.StatusInternalServerError)
		atomic.AddUint64(&h.metrics.failedRequests, 1)
		return
	}

	// Update metrics for successful request
	atomic.AddUint64(&h.metrics.successfulRequests, 1)
	h.metrics.mutex.Lock()
	h.metrics.totalLatency += time.Since(startTime)
	h.metrics.mutex.Unlock()

	// Send JSON response
	h.sendJSONResponse(w, result, http.StatusOK)
}

// sendJSONError sends a standardized error response in JSON format
func (h *Handler) sendJSONError(w http.ResponseWriter, errMsg string, statusCode int) {
	response := Response{
		Status:  "error",
		Message: errMsg,
		Code:    statusCode,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// sendJSONResponse sends a standardized success response in JSON format
func (h *Handler) sendJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	response := Response{
		Status: "success",
		Data:   data,
		Code:   statusCode,
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// handleJSONMessage processes a JSON message based on the method
func (h *Handler) handleJSONMessage(ctx context.Context, data []byte, methodName string) (interface{}, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty request body")
	}

	var result interface{}
	var err error

	// Handle methods
	switch methodName {
	case "register":
		result, err = h.handleRegister(ctx, data)
	case "login":
		result, err = h.handleLogin(ctx, data)
	default:
		return nil, fmt.Errorf("unknown method: %s", methodName)
	}

	if err != nil {
		return nil, err
	}

	return result, nil
}

// handleRegister processes registration requests
func (h *Handler) handleRegister(ctx context.Context, content []byte) (interface{}, error) {
	// Get user object from pool
	user := h.msgPool.Get().(*domain.User)
	defer h.msgPool.Put(user)
	*user = domain.User{} // Reset fields

	if err := json.Unmarshal(content, user); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	// Validate user data
	if user.Username == "" || user.Password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	if err := h.userUC.RegisterUser(ctx, user); err != nil {
		return nil, fmt.Errorf("registration failed: %v", err)
	}

	return map[string]interface{}{
		"message": "User registered successfully",
	}, nil
}

// handleLogin processes login requests
func (h *Handler) handleLogin(ctx context.Context, content []byte) (interface{}, error) {
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.Unmarshal(content, &credentials); err != nil {
		return nil, fmt.Errorf("invalid input data: %v", err)
	}

	if credentials.Username == "" || credentials.Password == "" {
		return nil, fmt.Errorf("missing username or password")
	}

	token, err := h.userUC.LoginUser(ctx, credentials.Username, credentials.Password)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %v", err)
	}

	return map[string]interface{}{
		"token": token,
	}, nil
}