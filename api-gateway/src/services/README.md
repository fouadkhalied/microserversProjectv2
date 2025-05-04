# ServiceClient for API Gateway

## Overview

ServiceClient is a high-performance TCP-based client for connecting to backend microservices from an API gateway. It implements a binary protocol for efficient communication, handles connection pooling, load balancing, reconnection logic, and performance monitoring.

## Features

- **Binary Protocol**: Custom lightweight binary protocol for efficient communication
- **Connection Pooling**: Maintains a pool of active connections to each service
- **Load Balancing**: Distributes requests across available connections
- **Automatic Reconnection**: Handles service disruptions with exponential backoff
- **Health Checks**: Monitors connection health and removes unhealthy connections
- **Performance Metrics**: Tracks request/response latency and throughput
- **Resource Management**: Efficiently manages idle connections and buffer resources

## Table of Contents

- [Binary Protocol](#binary-protocol)
- [Classes](#classes)
  - [CircularBuffer](#circularbuffer)
  - [ObjectPool](#objectpool)
  - [ServiceClient](#serviceclient)
- [Core Functionality](#core-functionality)
- [Configuration](#configuration)
- [Usage Example](#usage-example)

## Binary Protocol

The client implements a custom binary protocol (v1) with the following structure:

```
[
  Header (2 bytes): 0x55, 0x57 (UW magic bytes)
  Version (1 byte): 0x01 (protocol version)
  Request ID (16 bytes): UUID
  Method Length (1 byte): Length of method string
  Method (variable): UTF-8 encoded method name
  Content Length (4 bytes): Length of payload
  Content (variable): JSON payload
]
```

## Classes

### CircularBuffer

A specialized buffer implementation for efficient memory management when processing large data streams.

#### Methods:

- **constructor(capacity)**: Initializes buffer with specified capacity
- **write(data)**: Writes data to the buffer, resizing if necessary
- **peek(length)**: Retrieves data without removing it
- **read(length)**: Retrieves and removes data
- **skip(length)**: Skips ahead in the buffer
- **clear()**: Resets the buffer
- **resize(newCapacity)**: Increases buffer capacity
- **findPattern(pattern)**: Searches for a specific byte pattern in the buffer

### ObjectPool

A generic object pooling implementation to reduce garbage collection overhead.

#### Methods:

- **constructor(createFn, resetFn, initialSize, maxSize)**: Creates a pool of reusable objects
- **get()**: Retrieves an object from the pool or creates a new one
- **release(item)**: Returns an object to the pool

### ServiceClient

The main class that manages connections to services and handles request/response lifecycle.

#### Key Properties:

- **pendingRequests**: Map of in-flight requests awaiting responses
- **serviceConfigs**: Configuration settings for each service
- **metrics**: Performance metrics for each service
- **connections**: Active connections for each service

#### Core Methods:

- **constructor()**: Initializes the client with default services
- **configureService(serviceName, config)**: Configures a service with custom settings
- **request(serviceName, method, data)**: High-level method to send a request
- **sendBinaryRequest(serviceName, method, payload)**: Sends a request using the binary protocol
- **shutdown()**: Gracefully closes all connections

## Core Functionality

### Connection Management

- **createConnection(serviceName)**: Establishes a new connection to a service
- **closeConnection(serviceName, connection)**: Closes an existing connection
- **ensureMinConnections(serviceName)**: Maintains minimum required connections
- **getAvailableConnection(serviceName)**: Gets a connection using round-robin selection
- **performHealthChecks()**: Periodically checks connection health

### Request Processing

- **createBinaryRequest(method, payload, requestId)**: Creates a binary request packet
- **processResponseBuffer(serviceName, connection)**: Processes incoming response data
- **handleResponse(requestId, responseData, serviceName)**: Resolves pending requests

### Helper Methods

- **uuidToBytes(uuid, buffer, offset)**: Converts UUID string to bytes
- **bytesToUuid(bytes)**: Converts bytes to UUID string
- **pingConnection(serviceName, connection)**: Sends a ping to check connection health

### Metrics and Monitoring

- **createEmptyMetrics()**: Initializes performance metrics
- **calculateRequestsPerSecond()**: Updates performance metrics
- **resetMetrics()**: Resets performance metrics periodically
- **getMetrics(serviceName)**: Retrieves current metrics
- **getConnectionStatus()**: Returns connection pool status for monitoring

## Configuration

ServiceClient accepts the following configuration parameters for each service:

- **host**: Service hostname (default: 'localhost')
- **port**: Service port number
- **maxConnections**: Maximum number of connections (default: 10)
- **minConnections**: Minimum number of connections (default: 1)
- **timeout**: Request timeout in milliseconds (default: 5000)
- **healthCheckInterval**: Interval between health checks (default: 30000)
- **reconnectDelay**: Base delay for reconnection attempts (default: 1000)

## Usage Example

```typescript
// Create and configure the service client
const serviceClient = new ServiceClient();

// Configure a service
serviceClient.configureService('user-service', {
  host: 'users.internal',
  port: 3001,
  maxConnections: 20,
  minConnections: 5,
  timeout: 3000
});

// Send a request to the service
try {
  const response = await serviceClient.request(
    'user-service',
    'getUserProfile',
    { userId: '12345' }
  );
  console.log('User profile:', response);
} catch (error) {
  console.error('Request failed:', error);
}

// Get service metrics
const metrics = serviceClient.getMetrics('user-service');
console.log('Service metrics:', metrics);

// Get connection status
const status = serviceClient.getConnectionStatus();
console.log('Connection status:', status);

// Shutdown gracefully
await serviceClient.shutdown();
```

## Performance Considerations

- Uses TCP keepalive to detect dead connections
- Disables Nagle's algorithm for low-latency transmission
- Implements intelligent buffer management with CircularBuffer
- Uses object pooling to reduce garbage collection pressure
- Employs round-robin load balancing across connections
- Automatically closes idle connections after 5 minutes of inactivity
- Implements exponential backoff for reconnection attempts

## Error Handling

- Automatically reconnects after connection failures
- Detects and handles protocol parsing errors
- Uses timeouts to prevent stuck requests
- Emits events for monitoring connection state changes
- Gracefully handles service unavailability