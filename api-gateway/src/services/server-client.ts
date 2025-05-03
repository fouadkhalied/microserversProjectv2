// api-gateway/src/services/service-client.ts
import * as uWS from 'uWebSockets.js';
import { v4 as uuidv4 } from 'uuid';
import { performance } from 'perf_hooks';

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
}

interface ServiceConfig {
  url: string;
  maxConnections: number;
  timeout: number;
}

const DEFAULT_TIMEOUT = 5000; // 5 seconds

export class ServiceClient {
  private readonly MAGIC_BYTES = new Uint8Array([0x55, 0x57]); // "UW"
  private readonly PROTOCOL_VERSION = 0x01;
  private pendingRequests: Map<string, PendingRequest> = new Map();
  private serviceConfigs: Map<string, ServiceConfig> = new Map();
  private metrics: Map<string, PerformanceMetrics> = new Map();
  private activeConnections: Map<string, number> = new Map();
  
  constructor() {
    this.serviceConfigs.set('user-service', {
      url: 'http://localhost:3001',
      maxConnections: 100,
      timeout: 5000
    });
    
    // Initialize default metrics for each service
    this.serviceConfigs.forEach((_, serviceName) => {
      this.metrics.set(serviceName, this.createEmptyMetrics());
      this.activeConnections.set(serviceName, 0);
    });
    
    // Set up metrics reset interval (every hour)
    setInterval(() => this.resetMetrics(), 60 * 60 * 1000);
  }

  private createEmptyMetrics(): PerformanceMetrics {
    return {
      totalRequests: 0,
      successfulRequests: 0,
      failedRequests: 0,
      totalLatency: 0,
      maxLatency: 0,
      minLatency: Number.MAX_SAFE_INTEGER
    };
  }

  private resetMetrics() {
    this.serviceConfigs.forEach((_, serviceName) => {
      this.metrics.set(serviceName, this.createEmptyMetrics());
    });
    console.log('Performance metrics reset');
  }

  // Get performance metrics for a specific service or all services
  public getMetrics(serviceName?: string): Record<string, PerformanceMetrics> {
    if (serviceName && this.metrics.has(serviceName)) {
      return { [serviceName]: this.metrics.get(serviceName)! };
    }
    
    const result: Record<string, PerformanceMetrics> = {};
    this.metrics.forEach((metrics, name) => {
      result[name] = metrics;
    });
    return result;
  }

  // Send a binary request to a service
  async sendBinaryRequest(serviceName: string, method: string, payload: any): Promise<any> {
    // Check if service is configured
    if (!this.serviceConfigs.has(serviceName)) {
      throw new Error(`Service "${serviceName}" not configured`);
    }
    
    const serviceConfig = this.serviceConfigs.get(serviceName)!;
    const activeConnections = this.activeConnections.get(serviceName) || 0;
    
    // Check if we've reached the maximum connections for this service
    if (activeConnections >= serviceConfig.maxConnections) {
      throw new Error(`Maximum connections reached for service "${serviceName}"`);
    }
    
    // Increment active connections
    this.activeConnections.set(serviceName, activeConnections + 1);
    
    // Update metrics
    const serviceMetrics = this.metrics.get(serviceName)!;
    serviceMetrics.totalRequests++;
    
    // Generate a unique request ID
    const requestId = uuidv4();
    const startTime = performance.now();
    
    try {
      return await this.executeBinaryRequest(serviceName, method, payload, requestId, startTime);
    } catch (error) {
      // Update failure metrics
      serviceMetrics.failedRequests++;
      
      // Re-throw the error
      throw error;
    } finally {
      // Decrement active connections
      this.activeConnections.set(serviceName, this.activeConnections.get(serviceName)! - 1);
    }
  }
  
  private async executeBinaryRequest(
    serviceName: string, 
    method: string, 
    payload: any, 
    requestId: string,
    startTime: number
  ): Promise<any> {
    return new Promise((resolve, reject) => {
      const serviceConfig = this.serviceConfigs.get(serviceName)!;
      const requestIdBytes = this.uuidToBytes(requestId);
      
      // Convert method to bytes
      const methodBytes = new TextEncoder().encode(method);
      if (methodBytes.length > 255) {
        reject(new Error('Method name too long'));
        return;
      }
      
      // Convert payload to JSON bytes
      const payloadBytes = new TextEncoder().encode(JSON.stringify(payload));
      
      // Calculate the total message size
      const messageSize = 2 + 1 + 16 + 1 + methodBytes.length + 4 + payloadBytes.length;
      const message = new Uint8Array(messageSize);
      
      let offset = 0;
      
      // Add magic bytes
      message[offset++] = this.MAGIC_BYTES[0];
      message[offset++] = this.MAGIC_BYTES[1];
      
      // Add protocol version
      message[offset++] = this.PROTOCOL_VERSION;
      
      // Add request ID
      message.set(requestIdBytes, offset);
      offset += 16;
      
      // Add method length
      message[offset++] = methodBytes.length;
      
      // Add method
      message.set(methodBytes, offset);
      offset += methodBytes.length;
      
      // Add content length
      new DataView(message.buffer).setUint32(offset, payloadBytes.length, true);
      offset += 4;
      
      // Add content
      message.set(payloadBytes, offset);
      
      // Store the pending request
      const timer = setTimeout(() => {
        if (this.pendingRequests.has(requestId)) {
          this.pendingRequests.get(requestId)?.reject(new Error(`Request timeout after ${serviceConfig.timeout}ms`));
          this.pendingRequests.delete(requestId);
        }
      }, serviceConfig.timeout || DEFAULT_TIMEOUT);
      
      this.pendingRequests.set(requestId, {
        resolve,
        reject,
        timer,
        startTime
      });
      
      // Send the binary request using HTTP
      this.sendHttpBinaryRequest(serviceName, method, message, requestId)
        .catch(error => {
          if (this.pendingRequests.has(requestId)) {
            this.pendingRequests.get(requestId)?.reject(error);
            this.pendingRequests.delete(requestId);
            clearTimeout(timer);
          }
        });
    });
  }
  
  private async sendHttpBinaryRequest(serviceName: string, method: string, message: Uint8Array, requestId: string): Promise<void> {
    const serviceConfig = this.serviceConfigs.get(serviceName)!;
    
    try {
      // Fixed URL path to match the Go service expectations
      const response = await fetch(`${serviceConfig.url}/user/${method}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/octet-stream',
          'X-Request-ID': requestId,
          'Content-Length': message.length.toString() // Add explicit content length
        },
        body: message,
        // Make sure the entire message is sent as a single chunk
        duplex: 'half'
      });
      
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status} ${await response.text()}`);
      }
      
      // Read the binary response
      const responseBuffer = await response.arrayBuffer();
      this.handleBinaryResponse(new Uint8Array(responseBuffer), requestId, serviceName);
    } catch (error) {
      if (this.pendingRequests.has(requestId)) {
        const pendingRequest = this.pendingRequests.get(requestId)!;
        clearTimeout(pendingRequest.timer);
        pendingRequest.reject(error);
        this.pendingRequests.delete(requestId);
      }
      console.error(`Error sending request to ${serviceName}:`, error);
      throw error;
    }
  }
  
  private handleBinaryResponse(response: Uint8Array, expectedRequestId: string, serviceName: string): void {
    try {
      // Check magic bytes
      if (response[0] !== this.MAGIC_BYTES[0] || response[1] !== this.MAGIC_BYTES[1]) {
        throw new Error('Invalid response format: incorrect magic bytes');
      }
      
      // Check protocol version
      if (response[2] !== this.PROTOCOL_VERSION) {
        throw new Error(`Protocol version mismatch: expected ${this.PROTOCOL_VERSION}, got ${response[2]}`);
      }
      
      // Extract request ID
      const requestIdBytes = response.slice(3, 19);
      const requestId = this.bytesToUuid(requestIdBytes);
      
      // Verify request ID matches
      if (requestId !== expectedRequestId) {
        throw new Error('Response request ID does not match');
      }
      
      // Extract content length
      const contentLength = new DataView(response.buffer).getUint32(19, true);
      
      // Extract content
      const content = response.slice(23, 23 + contentLength);
      const jsonContent = new TextDecoder().decode(content);
      
      // Resolve the pending request
      if (this.pendingRequests.has(requestId)) {
        const pendingRequest = this.pendingRequests.get(requestId)!;
        clearTimeout(pendingRequest.timer);
        
        // Update metrics
        const endTime = performance.now();
        const latency = endTime - pendingRequest.startTime;
        const metrics = this.metrics.get(serviceName)!;
        
        metrics.successfulRequests++;
        metrics.totalLatency += latency;
        metrics.maxLatency = Math.max(metrics.maxLatency, latency);
        metrics.minLatency = Math.min(metrics.minLatency, latency);
        
        try {
          const responseData = JSON.parse(jsonContent);
          pendingRequest.resolve(responseData);
        } catch (error) {
          pendingRequest.reject(new Error('Invalid JSON response'));
        }
        
        this.pendingRequests.delete(requestId);
      }
    } catch (error) {
      console.error('Error handling binary response:', error);
      // If there's an error and we can't match to a specific request,
      // we can't do much but log it
    }
  }

  private uuidToBytes(uuid: string): Uint8Array {
    const bytes = new Uint8Array(16);
    const parts = uuid.replace(/-/g, '').match(/.{2}/g) || [];
    
    for (let i = 0; i < 16; i++) {
      bytes[i] = parseInt(parts[i], 16);
    }
    
    return bytes;
  }
  
  private bytesToUuid(bytes: Uint8Array): string {
    const hex = Array.from(bytes)
      .map(b => b.toString(16).padStart(2, '0'))
      .join('');
    
    return [
      hex.slice(0, 8),
      hex.slice(8, 12),
      hex.slice(12, 16),
      hex.slice(16, 20),
      hex.slice(20)
    ].join('-');
  }

  // Add or update a service configuration
  public configureService(name: string, config: ServiceConfig): void {
    this.serviceConfigs.set(name, config);
    
    // Initialize metrics if not already present
    if (!this.metrics.has(name)) {
      this.metrics.set(name, this.createEmptyMetrics());
      this.activeConnections.set(name, 0);
    }
  }
}