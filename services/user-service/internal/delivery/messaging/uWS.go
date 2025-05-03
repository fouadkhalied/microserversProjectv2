// internal/delivery/messaging/handler.go

package messaging

import (
	"context"
	"encoding/binary"
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
	// Binary protocol constants
	magicByte1      = 0x55 // 'U'
	magicByte2      = 0x57 // 'W'
	protocolVersion = 0x01 // Version 1
	headerSize      = 2    // Magic bytes
	versionSize     = 1    // Protocol version
	uuidSize        = 16   // Request ID
	methodLenSize   = 1    // Method name length
	contentLenSize  = 4    // Content length
	
	// Performance settings
	maxConcurrentRequests = 10000
	handlerTimeout        = 5 * time.Second
	rateLimitRequests     = 5000 // Requests per second
	rateLimitBurst        = 1000 // Burst capacity
)

// Handler manages binary message processing
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

// NewHandler creates a new binary message handler
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

// ServeHTTP handles HTTP requests with binary data
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Apply rate limiting
	if !h.limiter.Allow() {
		http.Error(w, "Too many requests", http.StatusTooManyRequests)
		return
	}

	// Increment active request counter
	if atomic.AddInt32(&h.activeRequests, 1) > maxConcurrentRequests {
		atomic.AddInt32(&h.activeRequests, -1) // Decrement counter
		http.Error(w, "Server overloaded", http.StatusServiceUnavailable)
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
		h.sendError(w, "Method not specified", http.StatusBadRequest, nil)
		atomic.AddUint64(&h.metrics.failedRequests, 1)
		return
	}

	// Read the binary request
	log.Printf("Reading binary request for method: %s", method)
	data, err := h.readBinaryRequest(r)
	if err != nil {
		log.Printf("Error reading request: %v", err)
		h.sendError(w, err.Error(), http.StatusBadRequest, nil)
		atomic.AddUint64(&h.metrics.failedRequests, 1)
		return
	}

	// Process the binary message
	requestID, response, err := h.handleBinaryMessage(ctx, data, method)
	if err != nil {
		log.Printf("Error handling message: %v", err)
		h.sendError(w, err.Error(), http.StatusInternalServerError, requestID)
		atomic.AddUint64(&h.metrics.failedRequests, 1)
		return
	}

	// Update metrics for successful request
	atomic.AddUint64(&h.metrics.successfulRequests, 1)
	h.metrics.mutex.Lock()
	h.metrics.totalLatency += time.Since(startTime)
	h.metrics.mutex.Unlock()

	// Send response
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(response)
}

func (h *Handler) readBinaryRequest(r *http.Request) ([]byte, error) {
	// Fixed: Properly read the entire request body
	if r.Body == nil {
		return nil, fmt.Errorf("request body is nil")
	}

	// Use io.ReadAll to properly read all data until EOF
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %v", err)
	}
	defer r.Body.Close()

	if len(data) == 0 {
		return nil, fmt.Errorf("empty request body")
	}

	// Log the size of the received data for debugging
	log.Printf("Received %d bytes of binary data", len(data))
	
	return data, nil
}

func (h *Handler) sendError(w http.ResponseWriter, errMsg string, statusCode int, requestID []byte) {
	// Check if the requestID is valid, if not use an empty one
	if requestID == nil {
		requestID = make([]byte, uuidSize)
	}
	
	errorData := map[string]interface{}{
		"status":  "error",
		"message": errMsg,
		"code":    statusCode,
	}
	
	jsonData, _ := json.Marshal(errorData)

	response := h.createBinaryResponse(requestID, jsonData)
	
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(statusCode)
	w.Write(response)
}

func (h *Handler) createBinaryResponse(requestID []byte, jsonData []byte) []byte {
	responseLen := headerSize + versionSize + uuidSize + contentLenSize + len(jsonData)
	response := make([]byte, responseLen)

	// Add magic bytes
	response[0] = magicByte1
	response[1] = magicByte2
	
	// Add protocol version
	response[2] = protocolVersion

	// Add request ID
	copy(response[headerSize+versionSize:], requestID)

	// Add content length
	binary.LittleEndian.PutUint32(response[headerSize+versionSize+uuidSize:], uint32(len(jsonData)))

	// Add content
	copy(response[headerSize+versionSize+uuidSize+contentLenSize:], jsonData)

	return response
}

// handleBinaryMessage processes a binary message
func (h *Handler) handleBinaryMessage(ctx context.Context, data []byte, methodName string) ([]byte, []byte, error) {
	// Check minimum message size
	minSize := headerSize + versionSize + uuidSize + methodLenSize
	if len(data) < minSize {
		return nil, nil, fmt.Errorf("message too short: got %d bytes, expected at least %d bytes", len(data), minSize)
	}

	// Verify magic bytes
	if data[0] != magicByte1 || data[1] != magicByte2 {
		return nil, nil, fmt.Errorf("invalid magic bytes: expected [%x, %x], got [%x, %x]", 
			magicByte1, magicByte2, data[0], data[1])
	}
	
	// Verify protocol version
	if data[2] != protocolVersion {
		return nil, nil, fmt.Errorf("unsupported protocol version: %d", data[2])
	}

	// Extract request ID
	offset := headerSize + versionSize
	requestID := data[offset : offset+uuidSize]
	offset += uuidSize

	// Extract method length
	methodLen := int(data[offset])
	offset += methodLenSize

	// Check if message has enough bytes for method
	if len(data) < offset+methodLen {
		return requestID, nil, fmt.Errorf("invalid method length: message too short")
	}
	
	// Extract method name from the message (for verification)
	method := string(data[offset : offset+methodLen])
	offset += methodLen
	
	// Verify method name matches URL path parameter
	if method != methodName {
		return requestID, nil, fmt.Errorf("method mismatch: got %s, expected %s", method, methodName)
	}

	// Extract content length
	if len(data) < offset+contentLenSize {
		return requestID, nil, fmt.Errorf("message too short for content length")
	}
	contentLen := binary.LittleEndian.Uint32(data[offset : offset+contentLenSize])
	offset += contentLenSize

	// Extract content
	if len(data) < offset+int(contentLen) {
		return requestID, nil, fmt.Errorf("message too short for content: expected %d more bytes, got %d", 
			contentLen, len(data)-offset)
	}
	content := data[offset : offset+int(contentLen)]

	var result interface{}
	var err error

	// Handle methods
	switch method {
	case "register":
		result, err = h.handleRegister(ctx, content)
	case "login":
		result, err = h.handleLogin(ctx, content)
	default:
		return requestID, nil, fmt.Errorf("unknown method: %s", method)
	}

	if err != nil {
		return requestID, nil, err
	}

	// Marshal response
	jsonData, err := json.Marshal(result)
	if err != nil {
		return requestID, nil, fmt.Errorf("error marshaling response: %v", err)
	}

	// Create response with same binary format
	response := h.createBinaryResponse(requestID, jsonData)

	return requestID, response, nil
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
		"status":  "success",
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
		"status": "success",
		"token":  token,
	}, nil
}