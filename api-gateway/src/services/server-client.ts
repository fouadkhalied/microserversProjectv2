// api-gateway/src/services/service-client.ts
import * as uWS from 'uWebSockets.js';
import { v4 as uuidv4 } from 'uuid';
import { performance } from 'perf_hooks';
import * as net from 'net';
import { EventEmitter } from 'events';

// Binary protocol v1 structure:
// [
//   Header (2 bytes): 0x55, 0x57 (UW magic bytes)
//   Version (1 byte): 0x01 (protocol version)
//   Request ID (16 bytes): UUID
//   Method Length (1 byte): Length of method string
//   Method (variable): UTF-8 encoded method name
//   Content Length (4 bytes): Length of payload
//   Content (variable): JSON payload
// ]

interface PendingRequest {
  resolve: (value: any) => void;
  reject: (reason: any) => void;
  timer: NodeJS.Timeout;
  startTime: number;
}

interface PerformanceMetrics {
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  totalLatency: number;
  maxLatency: number;
  minLatency: number;
  requestsPerSecond: number;
  lastCalculated: number;
  recentRequestCount: number;  // Number of requests in last window instead of array
  lastResetTimestamp: number;  // When metrics were last reset
}

interface ServiceConfig {
  host: string;
  port: number;
  maxConnections: number;
  minConnections: number;
  timeout: number;
  healthCheckInterval: number;
  reconnectDelay: number;
}

interface ServiceConnection {
  id: string;
  socket: net.Socket;
  isAvailable: boolean;
  responseBuffer: CircularBuffer;  // Use circular buffer
  lastUsed: number;
  requestCount: number;
  lastHealthCheck: number;
}

const DEFAULT_TIMEOUT = 5000; // 5 seconds
const DEFAULT_RECONNECT_DELAY = 1000; // 1 second
const DEFAULT_HEALTH_CHECK_INTERVAL = 30000; // 30 seconds
const MAX_BUFFER_SIZE = 10 * 1024 * 1024; // 10MB max response size
const MAX_SOCKET_IDLE_TIME = 300000; // 5 minutes
const HEALTH_CHECK_JITTER = 5000; // Add jitter to health checks

// Circular buffer for efficient buffer management
class CircularBuffer {
  private buffer: Buffer;
  private readOffset: number = 0;
  private writeOffset: number = 0;
  private _length: number = 0;
  
  constructor(capacity: number = 64 * 1024) {
    this.buffer = Buffer.allocUnsafe(capacity);
  }
  
  get length(): number {
    return this._length;
  }
  
  get capacity(): number {
    return this.buffer.length;
  }
  
  get availableSpace(): number {
    return this.capacity - this._length;
  }
  
  write(data: Buffer): void {
    // Ensure there's enough space
    if (data.length > this.availableSpace) {
      this.resize(Math.max(this.capacity * 2, this.capacity + data.length));
    }
    
    // Handle wrap-around writes
    const firstWrite = Math.min(data.length, this.capacity - this.writeOffset);
    data.copy(this.buffer, this.writeOffset, 0, firstWrite);
    
    if (firstWrite < data.length) {
      // Write remaining data at the beginning of the buffer
      data.copy(this.buffer, 0, firstWrite);
    }
    
    this.writeOffset = (this.writeOffset + data.length) % this.capacity;
    this._length += data.length;
  }
  
  peek(length: number): Buffer {
    if (length > this._length) {
      length = this._length;
    }
    
    // Create a new buffer for the result
    const result = Buffer.allocUnsafe(length);
    
    // Handle wrap-around reads
    const firstRead = Math.min(length, this.capacity - this.readOffset);
    this.buffer.copy(result, 0, this.readOffset, this.readOffset + firstRead);
    
    if (firstRead < length) {
      // Read remaining data from the beginning of the buffer
      this.buffer.copy(result, firstRead, 0, length - firstRead);
    }
    
    return result;
  }
  
  read(length: number): Buffer {
    const result = this.peek(length);
    
    this.readOffset = (this.readOffset + result.length) % this.capacity;
    this._length -= result.length;
    
    return result;
  }
  
  skip(length: number): void {
    if (length > this._length) {
      length = this._length;
    }
    
    this.readOffset = (this.readOffset + length) % this.capacity;
    this._length -= length;
  }
  
  clear(): void {
    this.readOffset = 0;
    this.writeOffset = 0;
    this._length = 0;
  }
  
  private resize(newCapacity: number): void {
    if (newCapacity <= this.capacity) {
      return;
    }
    
    const newBuffer = Buffer.allocUnsafe(newCapacity);
    
    if (this._length > 0) {
      // Copy existing data to the new buffer
      if (this.readOffset < this.writeOffset) {
        // Data doesn't wrap around
        this.buffer.copy(newBuffer, 0, this.readOffset, this.writeOffset);
      } else {
        // Data wraps around
        const firstPart = this.capacity - this.readOffset;
        this.buffer.copy(newBuffer, 0, this.readOffset, this.capacity);
        this.buffer.copy(newBuffer, firstPart, 0, this.writeOffset);
      }
    }
    
    this.buffer = newBuffer;
    this.readOffset = 0;
    this.writeOffset = this._length;
  }
  
  // Find pattern in buffer, returns -1 if not found
  findPattern(pattern: Buffer): number {
    if (pattern.length > this._length) {
      return -1;
    }
    
    for (let i = 0; i < this._length - pattern.length + 1; i++) {
      let matches = true;
      
      for (let j = 0; j < pattern.length; j++) {
        const bufferIndex = (this.readOffset + i + j) % this.capacity;
        if (this.buffer[bufferIndex] !== pattern[j]) {
          matches = false;
          break;
        }
      }
      
      if (matches) {
        return i;
      }
    }
    
    return -1;
  }
}

// Object pool for reusing objects
class ObjectPool<T> {
  private items: T[] = [];
  private createFn: () => T;
  private resetFn: (item: T) => void;
  private maxSize: number;
  
  constructor(createFn: () => T, resetFn: (item: T) => void, initialSize: number = 0, maxSize: number = 1000) {
    this.createFn = createFn;
    this.resetFn = resetFn;
    this.maxSize = maxSize;
    
    // Pre-allocate items
    for (let i = 0; i < initialSize; i++) {
      this.items.push(this.createFn());
    }
  }
  
  get(): T {
    if (this.items.length > 0) {
      return this.items.pop()!;
    }
    return this.createFn();
  }
  
  release(item: T): void {
    if (this.items.length < this.maxSize) {
      this.resetFn(item);
      this.items.push(item);
    }
  }
}

export class ServiceClient extends EventEmitter {
  private readonly MAGIC_BYTES = Buffer.from([0x55, 0x57]); // "UW"
  private readonly PROTOCOL_VERSION = 0x01;
  private pendingRequests: Map<string, PendingRequest> = new Map();
  private serviceConfigs: Map<string, ServiceConfig> = new Map();
  private metrics: Map<string, PerformanceMetrics> = new Map();
  private connections: Map<string, ServiceConnection[]> = new Map();
  private connectionAttempts: Map<string, number> = new Map();
  private requestBufferPool: ObjectPool<Buffer>;
  
  // LRU connection caching
  private connectionUsageOrder: Map<string, number> = new Map();
  private nextConnectionOrder: number = 0;
  
  // Connection balancing
  private lastUsedConnectionIndex: Map<string, number> = new Map();
  
  constructor() {
    super();
    
    // Initialize buffer pool
    this.requestBufferPool = new ObjectPool<Buffer>(
      () => Buffer.allocUnsafe(8192), // Initial size
      (buffer) => {/* No reset needed for buffers */},
      50,  // Initial pool size
      200   // Max pool size
    );
    
    // Configure default services
    this.configureService('user-service', {
      host: process.env.USER_SERVICE_HOST || 'localhost',
      port: 3001,
      maxConnections: 100,
      minConnections: 5,
      timeout: 5000,
      healthCheckInterval: DEFAULT_HEALTH_CHECK_INTERVAL,
      reconnectDelay: DEFAULT_RECONNECT_DELAY
    });
    
    // Initialize metrics
    this.serviceConfigs.forEach((_, serviceName) => {
      this.metrics.set(serviceName, this.createEmptyMetrics());
      this.connections.set(serviceName, []);
      this.connectionAttempts.set(serviceName, 0);
      this.lastUsedConnectionIndex.set(serviceName, 0);
    });
    
    // Initialize connection pools for each service
    this.initializeConnectionPools();
    
    // Set up metrics calculation interval (every second)
    setInterval(() => this.calculateRequestsPerSecond(), 1000);
    
    // Set up metrics reset interval (every hour)
    setInterval(() => this.resetMetrics(), 60 * 60 * 1000);
    
    // Set up connection health check interval (with jitter)
    setInterval(() => this.performHealthChecks(), 10000 + Math.floor(Math.random() * HEALTH_CHECK_JITTER));
  }

  private createEmptyMetrics(): PerformanceMetrics {
    return {
      totalRequests: 0,
      successfulRequests: 0,
      failedRequests: 0,
      totalLatency: 0,
      maxLatency: 0,
      minLatency: Number.MAX_SAFE_INTEGER,
      requestsPerSecond: 0,
      lastCalculated: Date.now(),
      recentRequestCount: 0,
      lastResetTimestamp: Date.now()
    };
  }

  private resetMetrics() {
    const now = Date.now();
    this.serviceConfigs.forEach((_, serviceName) => {
      const metrics = this.metrics.get(serviceName)!;
      metrics.totalRequests = 0;
      metrics.successfulRequests = 0;
      metrics.failedRequests = 0;
      metrics.totalLatency = 0;
      metrics.maxLatency = 0;
      metrics.minLatency = Number.MAX_SAFE_INTEGER;
      metrics.requestsPerSecond = 0;
      metrics.recentRequestCount = 0;
      metrics.lastResetTimestamp = now;
    });
    console.log('Performance metrics reset');
  }

  private calculateRequestsPerSecond() {
    const now = Date.now();
    
    this.metrics.forEach((metrics) => {
      // Calculate requests per second based on the recent request count
      metrics.requestsPerSecond = metrics.recentRequestCount;
      metrics.recentRequestCount = 0; // Reset for next second
      metrics.lastCalculated = now;
    });
  }

  // Get performance metrics for a specific service or all services
  public getMetrics(serviceName?: string): Record<string, PerformanceMetrics> {
    if (serviceName && this.metrics.has(serviceName)) {
      return { [serviceName]: { ...this.metrics.get(serviceName)! } };
    }
    
    const result: Record<string, PerformanceMetrics> = {};
    this.metrics.forEach((metrics, name) => {
      result[name] = { ...metrics };
    });
    return result;
  }

  private initializeConnectionPools(): void {
    this.serviceConfigs.forEach((config, serviceName) => {
      this.ensureMinConnections(serviceName);
    });
  }

  private ensureMinConnections(serviceName: string): void {
    const config = this.serviceConfigs.get(serviceName)!;
    const connectionList = this.connections.get(serviceName)!;
    
    const availableConnections = connectionList.filter(conn => conn.isAvailable).length;
    const connectionsNeeded = config.minConnections - availableConnections;
    
    if (connectionsNeeded > 0) {
      // Create multiple connections asynchronously but with slight delays
      for (let i = 0; i < connectionsNeeded; i++) {
        setTimeout(() => {
          this.createConnection(serviceName);
        }, i * 50); // Stagger connection creation slightly
      }
    }
  }

  private performHealthChecks(): void {
    const now = Date.now();
    
    this.serviceConfigs.forEach((config, serviceName) => {
      const connectionList = this.connections.get(serviceName)!;
      
      // Check for idle connections to close
      let idleConnectionsRemoved = 0;
      for (let i = connectionList.length - 1; i >= 0; i--) {
        const conn = connectionList[i];
        const idleTime = now - conn.lastUsed;
        
        // Close idle connections that exceed the maximum idle time
        // but ensure we maintain minimum connections
        if (idleTime > MAX_SOCKET_IDLE_TIME && connectionList.length > config.minConnections) {
          console.log(`Closing idle connection to ${serviceName} after ${idleTime}ms of inactivity`);
          this.closeConnection(serviceName, conn);
          connectionList.splice(i, 1);
          idleConnectionsRemoved++;
        }
      }
      
      // Only check a subset of connections each time to distribute load
      // and avoid checking connections that were recently used
      const connectionsToCheck = connectionList.filter(conn => {
        const timeSinceLastHealthCheck = now - (conn.lastHealthCheck || 0);
        return conn.isAvailable && 
               timeSinceLastHealthCheck > config.healthCheckInterval &&
               // Don't health check recently used connections
               now - conn.lastUsed > config.healthCheckInterval / 2;
      });
      
      // Limit the number of connections checked at once
      const maxConnectionsToCheck = Math.min(5, Math.ceil(connectionList.length * 0.2));
      connectionsToCheck.slice(0, maxConnectionsToCheck).forEach(conn => {
        this.pingConnection(serviceName, conn);
        conn.lastHealthCheck = now;
      });
      
      // Ensure we have the minimum number of connections if we removed idle ones
      if (idleConnectionsRemoved > 0) {
        this.ensureMinConnections(serviceName);
      }
    });
  }

  private pingConnection(serviceName: string, connection: ServiceConnection): void {
    try {
      const pingPayload = { timestamp: Date.now() };
      const requestId = uuidv4();
      
      // Create ping request
      const pingRequest = this.createBinaryRequest('ping', pingPayload, requestId);
      
      // Track this ping request with a short timeout
      const timer = setTimeout(() => {
        if (this.pendingRequests.has(requestId)) {
          this.pendingRequests.delete(requestId);
          
          // Mark connection as unavailable and attempt to reconnect
          console.warn(`Ping timeout for connection to ${serviceName}`);
          this.closeConnection(serviceName, connection);
          this.createConnection(serviceName);
        }
      }, 2000); // Short timeout for ping
      
      this.pendingRequests.set(requestId, {
        resolve: () => {
          // Ping successful
          clearTimeout(timer);
        },
        reject: () => {
          // Ping failed
          clearTimeout(timer);
        },
        timer,
        startTime: Date.now()
      });
      
      // Send ping through the connection
      connection.socket.write(pingRequest);
    } catch (error) {
      console.error(`Error pinging connection to ${serviceName}:`, error);
      
      // Close and recreate the connection
      this.closeConnection(serviceName, connection);
      this.createConnection(serviceName);
    }
  }

  private createConnection(serviceName: string): void {
    const config = this.serviceConfigs.get(serviceName)!;
    const connectionList = this.connections.get(serviceName)!;
    
    // Check if we've reached the maximum connections
    if (connectionList.length >= config.maxConnections) {
      console.warn(`Maximum connections reached for service ${serviceName}`);
      return;
    }
    
    // Increment connection attempts
    const attempts = (this.connectionAttempts.get(serviceName) || 0) + 1;
    this.connectionAttempts.set(serviceName, attempts);
    
    // Create connection ID
    const connectionId = `${serviceName}-${Date.now()}-${uuidv4().substr(0, 8)}`;
    
    try {
      // Create TCP socket
      const socket = new net.Socket();
      
      // Create connection object
      const connection: ServiceConnection = {
        id: connectionId,
        socket,
        isAvailable: false,
        responseBuffer: new CircularBuffer(),
        lastUsed: Date.now(),
        requestCount: 0,
        lastHealthCheck: 0
      };
      
      // Set up socket event handlers
      socket.on('connect', () => {
        console.log(`Connected to ${serviceName} (${connection.id})`);
        connection.isAvailable = true;
        
        // Reset connection attempts on successful connection
        this.connectionAttempts.set(serviceName, 0);
        
        // Add to connection list only after successful connection
        connectionList.push(connection);
        
        // Add to LRU tracking
        this.connectionUsageOrder.set(connection.id, this.nextConnectionOrder++);
        
        // Emit event for external monitoring
        this.emit('connection', { serviceName, connectionId, status: 'connected' });
      });
      
      socket.on('data', (data) => {
        // Update LRU tracking on data receipt
        this.connectionUsageOrder.set(connection.id, this.nextConnectionOrder++);
        connection.lastUsed = Date.now();
        
        // Add data to the circular buffer
        connection.responseBuffer.write(data);
        
        // Process complete messages from the buffer
        this.processResponseBuffer(serviceName, connection);
      });
      
      socket.on('error', (err) => {
        console.error(`Socket error for ${serviceName} (${connection.id}):`, err.message);
        connection.isAvailable = false;
        
        // Emit event for external monitoring
        this.emit('connection', { 
          serviceName, 
          connectionId: connection.id, 
          status: 'error',
          error: err.message 
        });
      });
      
      socket.on('close', () => {
        console.log(`Connection closed to ${serviceName} (${connection.id})`);
        connection.isAvailable = false;
        
        // Remove from the connection list
        const index = connectionList.findIndex(c => c.id === connection.id);
        if (index !== -1) {
          connectionList.splice(index, 1);
        }
        
        // Remove from LRU tracking
        this.connectionUsageOrder.delete(connection.id);
        
        // Attempt to reconnect with backoff
        const attempts = this.connectionAttempts.get(serviceName) || 0;
        const delay = Math.min(config.reconnectDelay * Math.pow(1.5, attempts), 30000); // Max 30 second delay
        
        setTimeout(() => {
          this.createConnection(serviceName);
        }, delay);
        
        // Emit event for external monitoring
        this.emit('connection', { 
          serviceName, 
          connectionId: connection.id, 
          status: 'closed' 
        });
      });
      
      // Connect to the service
      socket.connect(config.port, config.host);
      
      // Set TCP keepalive to detect dead connections
      socket.setKeepAlive(true, 30000);
      
      // Set TCP_NODELAY to disable Nagle's algorithm
      socket.setNoDelay(true);
    } catch (error) {
      console.error(`Error creating connection to ${serviceName}:`, error);
      
      // Attempt to reconnect with backoff
      const attempts = this.connectionAttempts.get(serviceName) || 0;
      const delay = Math.min(config.reconnectDelay * Math.pow(1.5, attempts), 30000); // Max 30 second delay
      
      setTimeout(() => {
        this.createConnection(serviceName);
      }, delay);
    }
  }

  private closeConnection(serviceName: string, connection: ServiceConnection): void {
    try {
      connection.isAvailable = false;
      connection.socket.destroy();
      
      // Remove from LRU tracking
      this.connectionUsageOrder.delete(connection.id);
    } catch (error) {
      console.error(`Error closing connection to ${serviceName}:`, error);
    }
  }

  private processResponseBuffer(serviceName: string, connection: ServiceConnection): void {
    // Minimum response size: 2 (magic) + 1 (version) + 16 (UUID) + 4 (content length)
    const MIN_RESPONSE_SIZE = 23;
    
    // Continue processing while we have enough data for a complete header
    while (connection.responseBuffer.length >= MIN_RESPONSE_SIZE) {
      // Check magic bytes by peeking first 2 bytes
      const header = connection.responseBuffer.peek(2);
      if (header[0] !== this.MAGIC_BYTES[0] || header[1] !== this.MAGIC_BYTES[1]) {
        console.error(`Invalid magic bytes in response from ${serviceName}`);
        
        // Use the efficient pattern search in the circular buffer
        const magicIndex = connection.responseBuffer.findPattern(this.MAGIC_BYTES);
        
        if (magicIndex === -1) {
          // No valid magic bytes found, discard the entire buffer
          connection.responseBuffer.clear();
          return;
        } else {
          // Discard data up to the magic bytes
          connection.responseBuffer.skip(magicIndex);
          continue;
        }
      }
      
      // Peek more of the header to check version and get content length
      if (connection.responseBuffer.length < 3) {
        // Not enough data yet
        return;
      }
      
      const versionByte = connection.responseBuffer.peek(3)[2];
      if (versionByte !== this.PROTOCOL_VERSION) {
        console.error(`Protocol version mismatch: expected ${this.PROTOCOL_VERSION}, got ${versionByte}`);
        
        // Skip past the invalid byte and continue
        connection.responseBuffer.skip(1);
        continue;
      }
      
      // Ensure we have enough data for the full header
      if (connection.responseBuffer.length < 23) {
        // Not enough data yet
        return;
      }
      
      // Extract header (using peek to avoid modifying the buffer yet)
      const fullHeader = connection.responseBuffer.peek(23);
      
      // Extract request ID (bytes 3-18)
      const requestIdBytes = fullHeader.slice(3, 19);
      const requestId = this.bytesToUuid(requestIdBytes);
      
      // Extract content length (bytes 19-22)
      const contentLength = fullHeader.readUInt32LE(19);
      
      // Check if content length is reasonable
      if (contentLength > MAX_BUFFER_SIZE) {
        console.error(`Excessive content length (${contentLength} bytes) in response from ${serviceName}`);
        
        // Skip past the invalid header and continue
        connection.responseBuffer.skip(3);
        continue;
      }
      
      // Check if we have the complete message
      const totalMessageLength = 23 + contentLength;
      if (connection.responseBuffer.length < totalMessageLength) {
        // Not enough data yet, wait for more
        return;
      }
      
      // Read and remove the header from the buffer
      connection.responseBuffer.skip(23);
      
      // Read and remove the content from the buffer
      const content = connection.responseBuffer.read(contentLength);
      
      // Process the response
      try {
        const jsonContent = content.toString('utf8');
        let responseData;
        
        try {
          responseData = JSON.parse(jsonContent);
        } catch (error) {
          console.error(`Invalid JSON in response from ${serviceName}:`, error);
          responseData = { error: 'Invalid JSON response' };
        }
        
        // Handle the response
        this.handleResponse(requestId, responseData, serviceName);
      } catch (error) {
        console.error(`Error processing response from ${serviceName}:`, error);
      }
    }
  }

  private handleResponse(requestId: string, responseData: any, serviceName: string): void {
    // Find the pending request
    const pendingRequest = this.pendingRequests.get(requestId);
    if (!pendingRequest) {
      // This could be a late response for a request that timed out
      return;
    }
    
    // Clear the timeout
    clearTimeout(pendingRequest.timer);
    
    // Update metrics
    const endTime = performance.now();
    const latency = endTime - pendingRequest.startTime;
    const metrics = this.metrics.get(serviceName)!;
    
    metrics.successfulRequests++;
    metrics.totalLatency += latency;
    metrics.maxLatency = Math.max(metrics.maxLatency, latency);
    metrics.minLatency = Math.min(metrics.minLatency, latency);
    
    // Resolve the promise
    pendingRequest.resolve(responseData);
    
    // Remove from pending requests
    this.pendingRequests.delete(requestId);
  }

  // Send a binary request to a service
  public async sendBinaryRequest(serviceName: string, method: string, payload: any): Promise<any> {
    // Check if service is configured
    if (!this.serviceConfigs.has(serviceName)) {
      throw new Error(`Service "${serviceName}" not configured`);
    }
    
    // Update metrics
    const serviceMetrics = this.metrics.get(serviceName)!;
    serviceMetrics.totalRequests++;
    serviceMetrics.recentRequestCount++;
    
    // Generate a unique request ID
    const requestId = uuidv4();
    const startTime = performance.now();
    
    try {
      // Get an available connection
      const connection = await this.getAvailableConnection(serviceName);
      if (!connection) {
        throw new Error(`No available connections for service "${serviceName}"`);
      }
      
      // Create the binary request
      const binaryRequest = this.createBinaryRequest(method, payload, requestId);
      
      // Return a promise that will be resolved when the response is received
      return new Promise((resolve, reject) => {
        const serviceConfig = this.serviceConfigs.get(serviceName)!;
        
        // Set up timeout
        const timer = setTimeout(() => {
          if (this.pendingRequests.has(requestId)) {
            this.pendingRequests.delete(requestId);
            reject(new Error(`Request timeout after ${serviceConfig.timeout}ms`));
            
            // Update metrics
            serviceMetrics.failedRequests++;
          }
        }, serviceConfig.timeout);
        
        // Store the pending request
        this.pendingRequests.set(requestId, {
          resolve,
          reject,
          timer,
          startTime
        });
        
        // Send the request
        try {
          // Update usage metrics
          connection.lastUsed = Date.now();
          connection.requestCount++;
          this.connectionUsageOrder.set(connection.id, this.nextConnectionOrder++);
          
          // Send the data
          connection.socket.write(binaryRequest);
        } catch (error) {
          // Handle socket write errors
          clearTimeout(timer);
          this.pendingRequests.delete(requestId);
          
          // Mark connection as unavailable
          connection.isAvailable = false;
          
          // Create a new connection
          this.createConnection(serviceName);
          
          // Update metrics
          serviceMetrics.failedRequests++;
          
          // Reject the promise
          reject(new Error(`Failed to send request: ${(error as Error).message}`));
        }
      });
    } catch (error) {
      // Update failure metrics
      serviceMetrics.failedRequests++;
      
      // Re-throw the error
      throw error;
    }
  }

  private async getAvailableConnection(serviceName: string): Promise<ServiceConnection | null> {
    const connectionList = this.connections.get(serviceName)!;
    let availableConnections = connectionList.filter(conn => conn.isAvailable);
    
    if (availableConnections.length === 0) {
      // No available connections, create a new one if possible
      const config = this.serviceConfigs.get(serviceName)!;
      if (connectionList.length < config.maxConnections) {
        this.createConnection(serviceName);
      }
      
      // Wait a short time for connections to be established
      if (connectionList.length === 0) {
        await new Promise(resolve => setTimeout(resolve, 50));
        availableConnections = connectionList.filter(conn => conn.isAvailable);
      }
      
      if (availableConnections.length === 0) {
        return null;
      }
    }
    
    // Use round-robin selection for better load balancing
    const serviceLastIndex = this.lastUsedConnectionIndex.get(serviceName) || 0;
    const nextIndex = (serviceLastIndex + 1) % availableConnections.length;
    this.lastUsedConnectionIndex.set(serviceName, nextIndex);
    
    return availableConnections[nextIndex];
  }

  private createBinaryRequest(method: string, payload: any, requestId: string): Buffer {
    // Serialize payload to JSON
    const jsonPayload = JSON.stringify(payload);
    const payloadBuffer = Buffer.from(jsonPayload, 'utf8');
    
    // Convert method to UTF-8 buffer
    const methodBuffer = Buffer.from(method, 'utf8');
    
    // Calculate total message size
    // Magic bytes (2) + Version (1) + UUID (16) + Method length (1) + Method + Content length (4) + Content
    const totalLength = 2 + 1 + 16 + 1 + methodBuffer.length + 4 + payloadBuffer.length;
    
    // Get buffer from pool or create a new one if too large
    let buffer: Buffer;
    if (totalLength <= 8192) {
      buffer = this.requestBufferPool.get();
      if (buffer.length < totalLength) {
        // If the pooled buffer is too small, create a new one
        buffer = Buffer.allocUnsafe(totalLength);
      }
    } else {
      // For large requests, create a dedicated buffer
      buffer = Buffer.allocUnsafe(totalLength);
    }
    
    let offset = 0;
    
    // Write magic bytes
    buffer[offset++] = this.MAGIC_BYTES[0];
    buffer[offset++] = this.MAGIC_BYTES[1];
    
    // Write protocol version
    buffer[offset++] = this.PROTOCOL_VERSION;
    
    // Write request ID (UUID)
    this.uuidToBytes(requestId, buffer, offset);
    offset += 16;
    
    // Write method length
    buffer[offset++] = methodBuffer.length;
    
    // Write method
    methodBuffer.copy(buffer, offset);
    offset += methodBuffer.length;
    
    // Write content length
    buffer.writeUInt32LE(payloadBuffer.length, offset);
    offset += 4;
    
    // Write content
    payloadBuffer.copy(buffer, offset);
    
    // If we used a buffer from the pool but only used part of it,
    // return a slice to avoid sending unnecessary data
    if (buffer.length > totalLength) {
      return buffer.slice(0, totalLength);
    }
    
    return buffer;
  }

  // Helper method to convert UUID string to bytes
  private uuidToBytes(uuid: string, buffer: Buffer, offset: number): void {
    // Remove hyphens from UUID
    const cleanUuid = uuid.replace(/-/g, '');
    
    // Convert hex string to bytes
    for (let i = 0; i < 16; i++) {
      const hexByte = cleanUuid.substr(i * 2, 2);
      buffer[offset + i] = parseInt(hexByte, 16);
    }
  }

  // Helper method to convert bytes to UUID string
  private bytesToUuid(bytes: Buffer): string {
    // Convert bytes to hex string
    let uuid = '';
    for (let i = 0; i < 16; i++) {
      const hexByte = bytes[i].toString(16).padStart(2, '0');
      uuid += hexByte;
    }
    
    // Insert hyphens to format as UUID
    return `${uuid.slice(0, 8)}-${uuid.slice(8, 12)}-${uuid.slice(12, 16)}-${uuid.slice(16, 20)}-${uuid.slice(20)}`;
  }

  // Configure a service with the given options
  public configureService(serviceName: string, config: Partial<ServiceConfig>): void {
    const existingConfig = this.serviceConfigs.get(serviceName);
    
    const newConfig: ServiceConfig = {
      host: config.host || (existingConfig?.host || 'localhost'),
      port: config.port || (existingConfig?.port || 0),
      maxConnections: config.maxConnections || (existingConfig?.maxConnections || 10),
      minConnections: config.minConnections || (existingConfig?.minConnections || 1),
      timeout: config.timeout || (existingConfig?.timeout || DEFAULT_TIMEOUT),
      healthCheckInterval: config.healthCheckInterval || (existingConfig?.healthCheckInterval || DEFAULT_HEALTH_CHECK_INTERVAL),
      reconnectDelay: config.reconnectDelay || (existingConfig?.reconnectDelay || DEFAULT_RECONNECT_DELAY)
    };
    
    this.serviceConfigs.set(serviceName, newConfig);
    
    // Initialize metrics if needed
    if (!this.metrics.has(serviceName)) {
      this.metrics.set(serviceName, this.createEmptyMetrics());
    }
    
    // Initialize connections list if needed
    if (!this.connections.has(serviceName)) {
      this.connections.set(serviceName, []);
    }
    
    // Initialize connection attempts counter if needed
    if (!this.connectionAttempts.has(serviceName)) {
      this.connectionAttempts.set(serviceName, 0);
    }
    
    // Initialize last used connection index if needed
    if (!this.lastUsedConnectionIndex.has(serviceName)) {
      this.lastUsedConnectionIndex.set(serviceName, 0);
    }
    
    // Ensure minimum connections are established
    this.ensureMinConnections(serviceName);
  }

  // Utility method to expose connection status for monitoring
  public getConnectionStatus(): Record<string, { total: number, available: number }> {
    const status: Record<string, { total: number, available: number }> = {};
    
    this.connections.forEach((connectionList, serviceName) => {
      const availableCount = connectionList.filter(conn => conn.isAvailable).length;
      status[serviceName] = {
        total: connectionList.length,
        available: availableCount
      };
    });
    
    return status;
  }

  // Graceful shutdown method
  public async shutdown(): Promise<void> {
    console.log('Shutting down ServiceClient');
    
    // Clear all timers
    this.pendingRequests.forEach((request) => {
      clearTimeout(request.timer);
      request.reject(new Error('Service client shutting down'));
    });
    
    // Close all connections
    for (const [serviceName, connectionList] of this.connections.entries()) {
      for (const connection of connectionList) {
        this.closeConnection(serviceName, connection);
      }
    }
    
    // Clear maps
    this.pendingRequests.clear();
    this.connections.clear();
    this.connectionUsageOrder.clear();
    
    console.log('ServiceClient shutdown complete');
  }

  // High-level method to send a request to a service
  public async request(serviceName: string, method: string, data: any): Promise<any> {
    return this.sendBinaryRequest(serviceName, method, data);
  }
}