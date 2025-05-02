// File: internal/delivery/websocket/handler.go

package messaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"time"
	"log"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"sync"
	"user-service/internal/domain"
	"user-service/internal/usecase"
)

const (
	// Binary protocol constants
	magicByte1     = 0x55 // 'U'
	magicByte2     = 0x57 // 'W'
	headerSize     = 2    // Magic bytes
	uuidSize       = 16   // Request ID
	methodLenSize  = 1    // Method name length
	contentLenSize = 4    // Content length
)

var (
	handlerTimeout = 2 * time.Second
)

// Handler manages WebSocket connections and processes binary messages
type Handler struct {
	userUC   *usecase.UserUsecase
	upgrader *websocket.Upgrader
	conns    map[*websocket.Conn]struct{}
	connsMu  sync.RWMutex
	msgPool  sync.Pool // Message object pool for reduced allocations
}

// NewHandler creates a new WebSocket handler
func NewHandler(userUC *usecase.UserUsecase) *Handler {
	return &Handler{
		userUC: userUC,
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
		conns: make(map[*websocket.Conn]struct{}),
		msgPool: sync.Pool{
			New: func() interface{} {
				return new(domain.User)
			},
		},
	}
}

// ServeHTTP handles HTTP requests and upgrades them to WebSocket connections
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Upgrade the HTTP connection to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading connection: %v", err)
		return
	}

	// Register the connection
	h.connsMu.Lock()
	h.conns[conn] = struct{}{}
	h.connsMu.Unlock()

	// Start handling messages in a goroutine
	go h.handleConnection(conn)
}

// handleConnection processes messages from a WebSocket connection
func (h *Handler) handleConnection(conn *websocket.Conn) {
	defer func() {
		// Unregister and close the connection when done
		h.connsMu.Lock()
		delete(h.conns, conn)
		h.connsMu.Unlock()
		conn.Close()
	}()

	for {
		// Read the next message
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error reading WebSocket message: %v", err)
			}
			break
		}

		// Only handle binary messages
		if messageType != websocket.BinaryMessage {
			log.Println("Received non-binary message, ignoring")
			continue
		}

		// Process the binary message in a goroutine
		go h.processBinaryMessage(conn, data)
	}
}

// processBinaryMessage handles binary protocol messages
func (h *Handler) processBinaryMessage(conn *websocket.Conn, data []byte) {
	// Validate message size and magic bytes
	if len(data) < headerSize+uuidSize+methodLenSize {
		log.Println("Binary message too short")
		return
	}

	if data[0] != magicByte1 || data[1] != magicByte2 {
		log.Println("Invalid magic bytes")
		return
	}

	// Extract request ID (16 bytes UUID)
	requestIDBytes := data[headerSize : headerSize+uuidSize]
	requestID, err := uuid.FromBytes(requestIDBytes)
	if err != nil {
		log.Printf("Invalid request ID: %v", err)
		return
	}

	// Extract method name length
	offset := headerSize + uuidSize
	methodLen := data[offset]
	offset += methodLenSize

	// Extract method name
	if len(data) < offset+int(methodLen) {
		log.Println("Invalid method length")
		return
	}
	method := string(data[offset : offset+int(methodLen)])
	offset += int(methodLen)

	// Extract content length
	if len(data) < offset+contentLenSize {
		log.Println("Message too short for content length")
		return
	}
	contentLen := binary.LittleEndian.Uint32(data[offset : offset+contentLenSize])
	offset += contentLenSize

	// Extract content
	if len(data) < offset+int(contentLen) {
		log.Println("Message too short for content")
		return
	}
	content := data[offset : offset+int(contentLen)]

	// Handle the method
	switch method {
	case "register":
		h.handleRegister(conn, requestID, content)
	case "login":
		h.handleLogin(conn, requestID, content)
	case "health":
		// Simple health check
		response := map[string]string{
			"status":    "healthy",
			"timestamp": time.Now().String(),
		}
		h.sendResponse(conn, requestID, response)
	default:
		log.Printf("Unknown method: %s", method)
		h.sendErrorResponse(conn, requestID, "unknown_method", "Method not supported")
	}
}

// handleRegister processes user registration requests
func (h *Handler) handleRegister(conn *websocket.Conn, requestID uuid.UUID, content []byte) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	// Get user object from pool to reduce allocations
	user := h.msgPool.Get().(*domain.User)
	defer h.msgPool.Put(user)

	// Reset fields to avoid data leakage between requests
	*user = domain.User{}

	// Decode user data
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields() // Adds some validation

	if err := decoder.Decode(user); err != nil {
		log.Println("Error decoding user:", err)
		h.sendErrorResponse(conn, requestID, "invalid_input", "Invalid input data")
		return
	}

	// Basic validation to fail fast before expensive operations
	if user.Username == "" || user.Password == "" || user.Email == "" {
		h.sendErrorResponse(conn, requestID, "missing_fields", "Missing required fields")
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
			if err.Error() == "username already exists" {
				h.sendErrorResponse(conn, requestID, "duplicate_username", "Username already exists")
			} else {
				h.sendErrorResponse(conn, requestID, "registration_failed", "Registration failed")
			}
			return
		}

		// Registration successful
		h.sendResponse(conn, requestID, map[string]string{
			"status":  "registered",
			"user_id": user.ID,
		})

	case <-ctx.Done():
		// Timeout occurred
		log.Println("Registration timed out")
		h.sendErrorResponse(conn, requestID, "timeout", "Registration timed out")
	}
}

// handleLogin processes user login requests
func (h *Handler) handleLogin(conn *websocket.Conn, requestID uuid.UUID, content []byte) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	// Get user object from pool to reduce allocations
	user := h.msgPool.Get().(*domain.User)
	defer h.msgPool.Put(user)

	// Reset fields to avoid data leakage between requests
	*user = domain.User{}

	if err := json.Unmarshal(content, user); err != nil {
		log.Println("Error decoding user:", err)
		h.sendErrorResponse(conn, requestID, "invalid_input", "Invalid input data")
		return
	}

	// Basic validation
	if user.Username == "" || user.Password == "" {
		h.sendErrorResponse(conn, requestID, "missing_fields", "Missing username or password")
		return
	}

	// Process login
	token, err := h.userUC.LoginUser(ctx, user.Username, user.Password)
	if err != nil {
		log.Println("Login failed:", err)
		h.sendErrorResponse(conn, requestID, "authentication_failed", "Login failed")
		return
	}

	// Login successful
	h.sendResponse(conn, requestID, map[string]string{
		"token":     token,
		"timestamp": time.Now().String(),
	})
}

// sendResponse sends a successful response back to the client
func (h *Handler) sendResponse(conn *websocket.Conn, requestID uuid.UUID, data interface{}) {
	// Marshal the response data
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		h.sendErrorResponse(conn, requestID, "internal_error", "Error generating response")
		return
	}

	// Create the binary response
	h.sendBinaryResponse(conn, requestID, jsonData)
}

// sendErrorResponse sends an error response back to the client
func (h *Handler) sendErrorResponse(conn *websocket.Conn, requestID uuid.UUID, code string, message string) {
	// Create error response
	errorResp := map[string]string{
		"error":   code,
		"message": message,
	}

	// Marshal the error response
	jsonData, err := json.Marshal(errorResp)
	if err != nil {
		log.Printf("Error marshaling error response: %v", err)
		return
	}

	// Send the binary response
	h.sendBinaryResponse(conn, requestID, jsonData)
}

// sendBinaryResponse formats and sends a binary response
func (h *Handler) sendBinaryResponse(conn *websocket.Conn, requestID uuid.UUID, jsonData []byte) {
	// Calculate the total message size
	totalSize := headerSize + uuidSize + contentLenSize + len(jsonData)

	// Create the response buffer
	response := make([]byte, totalSize)

	// Add magic bytes
	response[0] = magicByte1
	response[1] = magicByte2

	// Add request ID
	copy(response[headerSize:], requestID[:])

	// Add content length
	binary.LittleEndian.PutUint32(response[headerSize+uuidSize:], uint32(len(jsonData)))

	// Add content
	copy(response[headerSize+uuidSize+contentLenSize:], jsonData)

	// Write the response
	err := conn.WriteMessage(websocket.BinaryMessage, response)
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// Start starts the WebSocket server
func (h *Handler) Start(address string) error {
	log.Printf("Starting WebSocket server on %s", address)
	return http.ListenAndServe(address, h)
}

// Close closes all open WebSocket connections
func (h *Handler) Close() {
	h.connsMu.Lock()
	defer h.connsMu.Unlock()

	for conn := range h.conns {
		conn.Close()
	}
}
