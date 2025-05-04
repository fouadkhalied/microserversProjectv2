// internal/delivery/tcp/handler.go

package tcp

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"user-service/internal/domain"
	"user-service/internal/usecase"

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
	maxBufferSize         = 10 * 1024 * 1024 // 10MB max buffer size
	
	// Worker pool settings
	workerPoolSize       = 100 // Number of worker goroutines
	messageQueueSize     = 1000 // Queue depth for message processing
	connectionPoolSize   = 1000 // Number of concurrent connections to accept
)

// Message represents a work item for processing
type Message struct {
	conn      net.Conn
	data      []byte
	timestamp time.Time
}

// TCPHandler manages TCP binary message processing
type TCPHandler struct {
	userUC           *usecase.UserUsecase
	msgPool          sync.Pool // Message object pool for reduced allocations
	bufferPool       sync.Pool // Buffer pool for reuse
	activeRequests   int32     // Atomic counter for active requests
	limiter          *rate.Limiter
	metrics          *Metrics
	listener         net.Listener
	done             chan struct{}
	wg               sync.WaitGroup
	messageQueue     chan Message // Queue for message processing
	connectionSemaphore chan struct{} // Semaphore for connection limiting
}

// Metrics tracks performance data
type Metrics struct {
	totalRequests      uint64
	successfulRequests uint64
	failedRequests     uint64
	totalLatency       int64 // Nanoseconds
	avgLatency         int64 // Exponential moving average (updated atomically)
	startTime          time.Time
}

// NewTCPHandler creates a new TCP binary message handler
func NewTCPHandler(userUC *usecase.UserUsecase) *TCPHandler {
	h := &TCPHandler{
		userUC: userUC,
		msgPool: sync.Pool{
			New: func() interface{} {
				return new(domain.User)
			},
		},
		bufferPool: sync.Pool{
			New: func() interface{} {
				// Pre-allocate buffers of 4KB
				return make([]byte, 0, 4096)
			},
		},
		limiter: rate.NewLimiter(rate.Limit(rateLimitRequests), rateLimitBurst),
		metrics: &Metrics{
			startTime: time.Now(),
		},
		done:                make(chan struct{}),
		messageQueue:        make(chan Message, messageQueueSize),
		connectionSemaphore: make(chan struct{}, connectionPoolSize),
	}
	
	return h
}

// GetMetrics returns current metrics - lock-free implementation
func (h *TCPHandler) GetMetrics() map[string]interface{} {
	uptime := time.Since(h.metrics.startTime)
	totalReqs := atomic.LoadUint64(&h.metrics.totalRequests)
	successReqs := atomic.LoadUint64(&h.metrics.successfulRequests)
	failedReqs := atomic.LoadUint64(&h.metrics.failedRequests)
	avgLatency := time.Duration(atomic.LoadInt64(&h.metrics.avgLatency))
	
	return map[string]interface{}{
		"totalRequests":      totalReqs,
		"successfulRequests": successReqs,
		"failedRequests":     failedReqs,
		"avgLatencyMs":       avgLatency.Milliseconds(),
		"activeRequests":     atomic.LoadInt32(&h.activeRequests),
		"uptimeSeconds":      uptime.Seconds(),
		"requestsPerSecond":  float64(totalReqs) / uptime.Seconds(),
		"queueDepth":         len(h.messageQueue),
	}
}

// Start begins listening for TCP connections
func (h *TCPHandler) Start(address string) error {
	var err error
	h.listener, err = net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to start TCP listener: %v", err)
	}
	
	log.Printf("TCP server listening on %s", address)
	
	// Start worker pool
	numWorkers := runtime.GOMAXPROCS(0) * 2
	if numWorkers < workerPoolSize {
		numWorkers = workerPoolSize
	}
	
	for i := 0; i < numWorkers; i++ {
		h.wg.Add(1)
		go h.startWorker()
	}
	
	// Start multiple acceptors for better performance under high connection load
	acceptorCount := runtime.GOMAXPROCS(0)
	for i := 0; i < acceptorCount; i++ {
		h.wg.Add(1)
		go h.acceptConnections()
	}
	
	return nil
}

// Stop stops the TCP server
func (h *TCPHandler) Stop() error {
	close(h.done)
	
	if h.listener != nil {
		if err := h.listener.Close(); err != nil {
			return fmt.Errorf("error closing listener: %v", err)
		}
	}
	
	h.wg.Wait()
	close(h.messageQueue)
	log.Println("TCP server stopped")
	return nil
}

// acceptConnections handles incoming client connections
func (h *TCPHandler) acceptConnections() {
	defer h.wg.Done()
	
	for {
		select {
		case <-h.done:
			return
		case h.connectionSemaphore <- struct{}{}: // Acquire connection slot
			conn, err := h.listener.Accept()
			if err != nil {
				<-h.connectionSemaphore // Release on error
				select {
				case <-h.done:
					return
				default:
					log.Printf("Error accepting connection: %v", err)
					time.Sleep(time.Millisecond * 10) // Reduced backoff time
					continue
				}
			}
			
			h.wg.Add(1)
			go func() {
				defer h.wg.Done()
				defer func() { <-h.connectionSemaphore }() // Release connection slot when done
				h.handleConnection(conn)
			}()
		}
	}
}

// handleConnection processes data from a single client connection
func (h *TCPHandler) handleConnection(conn net.Conn) {
	defer conn.Close()
	
	// TCP_NODELAY disables Nagle's algorithm for better latency
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
		// Also consider TCP_QUICKACK if available in your environment
	}
	
	// Set connection timeout
	conn.SetDeadline(time.Now().Add(time.Minute * 10))
	
	// Get buffer from pool
	buffer := h.bufferPool.Get().([]byte)
	buffer = buffer[:0] // Reset length while keeping capacity
	defer h.bufferPool.Put(buffer)
	
	// Temporary buffer for reading - allocate once
	readBuffer := make([]byte, 16384) // Increased read buffer for fewer syscalls
	
	for {
		select {
		case <-h.done:
			return
		default:
			// Update read deadline for each read attempt
			conn.SetReadDeadline(time.Now().Add(time.Second * 60))
			
			n, err := conn.Read(readBuffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("Error reading from connection: %v", err)
				}
				return
			}
			
			if n == 0 {
				continue
			}
			
			// Append data to buffer
			buffer = append(buffer, readBuffer[:n]...)
			
			// Check buffer size to prevent memory attacks
			if len(buffer) > maxBufferSize {
				log.Printf("Buffer size exceeded for client %s", conn.RemoteAddr())
				return
			}
			
			// Process complete messages
			var processed int
			for processed < len(buffer) {
				msgSize, complete, err := h.checkMessageComplete(buffer[processed:])
				if err != nil {
					log.Printf("Error checking message: %v", err)
					return
				}
				
				if !complete {
					break
				}
				
				// Copy message data to avoid race conditions when multiple messages 
				// are processed from the same buffer
				msgData := make([]byte, msgSize)
				copy(msgData, buffer[processed:processed+msgSize])
				processed += msgSize
				
				// Apply rate limiting here to avoid queueing unnecessary messages
				if !h.limiter.Allow() {
					h.sendError(conn, "Rate limit exceeded", extractRequestID(msgData))
					continue
				}
				
				// Check if we can handle more requests
				if atomic.LoadInt32(&h.activeRequests) > maxConcurrentRequests {
					h.sendError(conn, "Server overloaded", extractRequestID(msgData))
					continue
				}
				
				// Send message to worker pool
				select {
				case h.messageQueue <- Message{
					conn:      conn,
					data:      msgData,
					timestamp: time.Now(),
				}:
					// Message queued successfully
				default:
					// Queue is full, send error to client
					h.sendError(conn, "Server busy, try again later", extractRequestID(msgData))
				}
			}
			
			// Keep unprocessed data in buffer
			if processed > 0 {
				if processed < len(buffer) {
					// Use copy to avoid memory leaks from large buffers
					copy(buffer, buffer[processed:])
					buffer = buffer[:len(buffer)-processed]
				} else {
					buffer = buffer[:0] // Reset buffer but keep capacity
				}
			}
		}
	}
}

// startWorker runs a worker goroutine that processes messages from the queue
func (h *TCPHandler) startWorker() {
	defer h.wg.Done()
	
	for {
		select {
		case <-h.done:
			return
		case msg, ok := <-h.messageQueue:
			if !ok {
				return // Channel closed
			}
			
			// Track active requests
			atomic.AddInt32(&h.activeRequests, 1)
			atomic.AddUint64(&h.metrics.totalRequests, 1)
			
			startTime := time.Now()
			
			// Process the message with a timeout context
			ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
			requestID, response, err := h.handleBinaryMessage(ctx, msg.data)
			cancel()
			
			if err != nil {
				h.sendError(msg.conn, err.Error(), requestID)
				atomic.AddUint64(&h.metrics.failedRequests, 1)
			} else {
				// Update metrics for successful request - lock-free
				atomic.AddUint64(&h.metrics.successfulRequests, 1)
				
				// Update latency metrics with exponential moving average
				latency := time.Since(startTime).Nanoseconds()
				h.updateAvgLatency(latency)
				
				// Set write deadline
				msg.conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
				
				// Send response
				_, err = msg.conn.Write(response)
				if err != nil {
					log.Printf("Error writing response: %v", err)
				}
			}
			
			// Decrement active requests
			atomic.AddInt32(&h.activeRequests, -1)
		}
	}
}

// updateAvgLatency updates the average latency using a lock-free exponential moving average
func (h *TCPHandler) updateAvgLatency(newLatency int64) {
	const alpha = 0.05 // Smoothing factor
	for {
		currentAvg := atomic.LoadInt64(&h.metrics.avgLatency)
		// EMA formula: newAvg = alpha * newValue + (1 - alpha) * currentAvg
		newAvg := int64(float64(newLatency)*alpha + float64(currentAvg)*(1-alpha))
		if atomic.CompareAndSwapInt64(&h.metrics.avgLatency, currentAvg, newAvg) {
			break
		}
		// If CAS failed, try again with the updated value
	}
}

// extractRequestID gets the request ID from a message
func extractRequestID(data []byte) []byte {
	if len(data) < headerSize+versionSize+uuidSize {
		return nil
	}
	offset := headerSize + versionSize
	return data[offset : offset+uuidSize]
}

// checkMessageComplete checks if a complete message is available in the buffer
func (h *TCPHandler) checkMessageComplete(buffer []byte) (int, bool, error) {
	// Check minimum header size
	if len(buffer) < headerSize+versionSize+uuidSize+methodLenSize {
		return 0, false, nil
	}
	
	// Verify magic bytes
	if buffer[0] != magicByte1 || buffer[1] != magicByte2 {
		return 0, false, fmt.Errorf("invalid magic bytes")
	}
	
	// Verify protocol version
	if buffer[2] != protocolVersion {
		return 0, false, fmt.Errorf("unsupported protocol version: %d", buffer[2])
	}
	
	// Method length is at offset headerSize+versionSize+uuidSize
	offset := headerSize + versionSize + uuidSize
	methodLen := int(buffer[offset])
	offset += methodLenSize
	
	// Check if we have enough bytes for the method name
	if len(buffer) < offset+methodLen {
		return 0, false, nil
	}
	
	// Move offset past method name
	offset += methodLen
	
	// Check if we have enough bytes for content length
	if len(buffer) < offset+contentLenSize {
		return 0, false, nil
	}
	
	// Extract content length
	contentLen := binary.LittleEndian.Uint32(buffer[offset:offset+contentLenSize])
	offset += contentLenSize
	
	// Calculate total message size
	totalSize := offset + int(contentLen)
	
	// Check if the buffer contains the complete message
	if len(buffer) < totalSize {
		return 0, false, nil
	}
	
	return totalSize, true, nil
}

func (h *TCPHandler) sendError(conn net.Conn, errMsg string, requestID []byte) {
	// Check if the requestID is valid, if not use an empty one
	if requestID == nil {
		requestID = make([]byte, uuidSize)
	}
	
	errorData := map[string]string{
		"status":  "error",
		"message": errMsg,
	}
	
	jsonData, _ := json.Marshal(errorData)

	response := h.createBinaryResponse(requestID, jsonData)
	
	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	
	// Send error response
	_, err := conn.Write(response)
	if err != nil {
		log.Printf("Error writing error response: %v", err)
	}
}

func (h *TCPHandler) createBinaryResponse(requestID []byte, jsonData []byte) []byte {
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
func (h *TCPHandler) handleBinaryMessage(ctx context.Context, data []byte) ([]byte, []byte, error) {
	// Check minimum message size
	minSize := headerSize + versionSize + uuidSize + methodLenSize
	if len(data) < minSize {
		return nil, nil, fmt.Errorf("message too short: got %d bytes, expected at least %d bytes", len(data), minSize)
	}
	
	// Extract request ID
	offset := headerSize + versionSize
	requestID := data[offset : offset+uuidSize]
	offset += uuidSize

	// Extract method length
	methodLen := int(data[offset])
	offset += methodLenSize

	// Extract method name
	method := string(data[offset : offset+methodLen])
	offset += methodLen

	// Extract content length
	contentLen := binary.LittleEndian.Uint32(data[offset : offset+contentLenSize])
	offset += contentLenSize

	// Extract content
	content := data[offset : offset+int(contentLen)]

	var result interface{}
	var err error

	// Handle methods
	switch method {
	case "register":
		result, err = h.handleRegister(ctx, content)
	case "login":
		result, err = h.handleLogin(ctx, content)
	case "ping":
		// Fast path for ping - no need for map allocation
		result = struct {
			Status string `json:"status"`
			Pong   int64  `json:"pong"`
		}{
			Status: "success",
			Pong:   time.Now().UnixNano() / int64(time.Millisecond),
		}
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
func (h *TCPHandler) handleRegister(ctx context.Context, content []byte) (interface{}, error) {
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

	// Use a struct instead of map for better performance
	return struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}{
		Status:  "success",
		Message: "User registered successfully",
	}, nil
}

// handleLogin processes login requests
func (h *TCPHandler) handleLogin(ctx context.Context, content []byte) (interface{}, error) {
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

	// Use a struct instead of map for better performance
	return struct {
		Status string `json:"status"`
		Token  string `json:"token"`
	}{
		Status: "success",
		Token:  token,
	}, nil
}