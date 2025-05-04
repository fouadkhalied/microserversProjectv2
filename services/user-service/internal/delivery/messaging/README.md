# TCP Binary Protocol Handler

This package implements a high-performance TCP server for handling binary protocol messages in a user service application. It's designed for low-latency, high-throughput message processing with features like connection pooling, worker pools, and rate limiting.

## Overview

The `tcp` package provides a TCP handler that:
- Manages incoming TCP connections using binary protocol
- Processes user registration and login requests
- Implements performance optimizations including:
  - Goroutine worker pools
  - Connection pooling
  - Message queuing
  - Object pooling
  - Rate limiting
  - Buffer reuse

## Protocol Specification

The binary protocol follows this format:

```
+------------+---------+----------+------------+--------+-------------+
| Magic (2B) | Ver(1B) | UUID(16B)| Method Len | Method | Content Len | Content |
+------------+---------+----------+------------+--------+-------------+
```

- **Magic Bytes**: `0x55 0x57` (ASCII "UW")
- **Protocol Version**: Currently `0x01`
- **UUID**: 16-byte request identifier
- **Method Length**: 1 byte indicating method name length
- **Method**: Method name (e.g., "register", "login")
- **Content Length**: 4 bytes (little-endian)
- **Content**: JSON-encoded payload

## Core Components

### TCPHandler

The main handler that manages all TCP connections and message processing.

```go
type TCPHandler struct {
    userUC             *usecase.UserUsecase
    msgPool            sync.Pool
    bufferPool         sync.Pool
    activeRequests     int32
    limiter            *rate.Limiter
    metrics            *Metrics
    listener           net.Listener
    done               chan struct{}
    wg                 sync.WaitGroup
    messageQueue       chan Message
    connectionSemaphore chan struct{}
}
```

### Message

Represents a work item for processing.

```go
type Message struct {
    conn      net.Conn
    data      []byte
    timestamp time.Time
}
```

### Metrics

Tracks performance data for the handler.

```go
type Metrics struct {
    totalRequests      uint64
    successfulRequests uint64
    failedRequests     uint64
    totalLatency       int64
    avgLatency         int64
    startTime          time.Time
}
```

## Function Documentation

### Constructor

#### `NewTCPHandler(userUC *usecase.UserUsecase) *TCPHandler`

Creates a new TCP handler with initialized pools, rate limiter and channels.

- **Parameters**:
  - `userUC`: User usecase implementation for business logic
- **Returns**: Initialized TCP handler

### Server Management

#### `Start(address string) error`

Starts the TCP server on the specified address.

- **Parameters**:
  - `address`: Network address to listen on (e.g., ":8080")
- **Returns**: Error if server fails to start
- **Actions**:
  - Creates TCP listener
  - Starts worker goroutines
  - Starts connection acceptor goroutines

#### `Stop() error`

Gracefully stops the TCP server.

- **Returns**: Error if shutdown fails
- **Actions**:
  - Signals all goroutines to stop
  - Closes the listener
  - Waits for all goroutines to finish

### Connection Handling

#### `acceptConnections()`

Accepts incoming TCP connections and manages connection limits.

- **Actions**:
  - Accepts connections from the listener
  - Uses connection semaphore to limit concurrent connections
  - Spawns a handler goroutine for each connection

#### `handleConnection(conn net.Conn)`

Handles a single TCP connection, reading and processing messages.

- **Parameters**:
  - `conn`: TCP connection to handle
- **Actions**:
  - Sets TCP optimization flags (TCP_NODELAY)
  - Reads data from connection
  - Processes complete messages
  - Handles rate limiting
  - Queues messages for worker processing

### Message Processing

#### `startWorker()`

Worker goroutine that processes messages from the queue.

- **Actions**:
  - Takes messages from the queue
  - Processes messages with a timeout
  - Sends responses back to clients
  - Updates metrics

#### `handleBinaryMessage(ctx context.Context, data []byte) ([]byte, []byte, error)`

Processes a binary message and generates a response.

- **Parameters**:
  - `ctx`: Context for the operation
  - `data`: Binary message data
- **Returns**:
  - Request ID
  - Response data
  - Error if processing failed
- **Actions**:
  - Parses the message format
  - Routes to appropriate handler based on method
  - Creates binary response

#### `handleRegister(ctx context.Context, content []byte) (interface{}, error)`

Handles user registration requests.

- **Parameters**:
  - `ctx`: Context for the operation
  - `content`: JSON-encoded registration data
- **Returns**:
  - Response object
  - Error if registration failed
- **Actions**:
  - Unmarshals JSON to user object
  - Validates user data
  - Calls user usecase to register user

#### `handleLogin(ctx context.Context, content []byte) (interface{}, error)`

Handles user login requests.

- **Parameters**:
  - `ctx`: Context for the operation
  - `content`: JSON-encoded login credentials
- **Returns**:
  - Response object with authentication token
  - Error if login failed
- **Actions**:
  - Unmarshals JSON to credentials
  - Validates credentials
  - Calls user usecase to authenticate user

### Helper Functions

#### `checkMessageComplete(buffer []byte) (int, bool, error)`

Checks if a buffer contains a complete message.

- **Parameters**:
  - `buffer`: Buffer to check
- **Returns**:
  - Message size
  - Boolean indicating if message is complete
  - Error if message format is invalid
- **Actions**:
  - Verifies magic bytes and protocol version
  - Checks if buffer contains complete message

#### `createBinaryResponse(requestID []byte, jsonData []byte) []byte`

Creates a binary response message.

- **Parameters**:
  - `requestID`: Original request ID
  - `jsonData`: JSON response data
- **Returns**: Formatted binary response

#### `sendError(conn net.Conn, errMsg string, requestID []byte)`

Sends an error response to the client.

- **Parameters**:
  - `conn`: Connection to send error to
  - `errMsg`: Error message
  - `requestID`: Original request ID
- **Actions**:
  - Creates error response
  - Sends response to client

#### `updateAvgLatency(newLatency int64)`

Updates the average latency metric using exponential moving average.

- **Parameters**:
  - `newLatency`: New latency measurement in nanoseconds
- **Actions**:
  - Updates average latency using lock-free atomic operations

#### `GetMetrics() map[string]interface{}`

Returns current performance metrics.

- **Returns**: Map of metrics including:
  - Total requests
  - Successful requests
  - Failed requests
  - Average latency
  - Active requests
  - Uptime
  - Requests per second
  - Queue depth

## Performance Considerations

- **Connection Pooling**: Limits number of concurrent connections
- **Worker Pool**: Processes messages in parallel
- **Rate Limiting**: Prevents server overload
- **Object Pooling**: Reduces GC pressure by reusing objects
- **Buffer Reuse**: Minimizes memory allocations
- **Lock-Free Metrics**: Uses atomic operations for contention-free metrics
- **TCP Optimizations**: Disables Nagle's algorithm for lower latency

## Configuration Constants

- `magicByte1`, `magicByte2`: Protocol magic bytes
- `protocolVersion`: Binary protocol version
- `maxConcurrentRequests`: Maximum concurrent requests
- `handlerTimeout`: Request processing timeout
- `rateLimitRequests`: Rate limit in requests per second
- `rateLimitBurst`: Burst capacity for rate limiting
- `maxBufferSize`: Maximum allowed message size
- `workerPoolSize`: Number of worker goroutines
- `messageQueueSize`: Queue depth for message processing
- `connectionPoolSize`: Maximum concurrent connections