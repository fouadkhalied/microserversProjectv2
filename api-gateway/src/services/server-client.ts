// api-gateway/src/services/service-client.ts
import { v4 as uuidv4 } from 'uuid';
import { performance } from 'perf_hooks';

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

  // Send a request to a service
  async sendRequest(serviceName: string, method: string, payload: any): Promise<any> {
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
      return await this.executeRequest(serviceName, method, payload, requestId, startTime);
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
  
  private async executeRequest(
    serviceName: string, 
    method: string, 
    payload: any, 
    requestId: string,
    startTime: number
  ): Promise<any> {
    return new Promise((resolve, reject) => {
      const serviceConfig = this.serviceConfigs.get(serviceName)!;
      
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
      
      // Send the HTTP request
      this.sendHttpJsonRequest(serviceName, method, payload, requestId)
        .catch(error => {
          if (this.pendingRequests.has(requestId)) {
            this.pendingRequests.get(requestId)?.reject(error);
            this.pendingRequests.delete(requestId);
            clearTimeout(timer);
          }
        });
    });
  }
  
  private async sendHttpJsonRequest(serviceName: string, method: string, payload: any, requestId: string): Promise<void> {
    const serviceConfig = this.serviceConfigs.get(serviceName)!;
    
    try {
      const response = await fetch(`${serviceConfig.url}/user/${method}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Request-ID': requestId,
        },
        body: JSON.stringify(payload)
      });
      
      if (!response.ok) {
        let errorText = await response.text();
        try {
          // Try to parse error as JSON
          const errorJson = JSON.parse(errorText);
          throw new Error(errorJson.message || `HTTP error: ${response.status}`);
        } catch (parseError) {
          throw new Error(`HTTP error: ${response.status} ${errorText}`);
        }
      }
      
      // Handle the response
      const responseData = await response.json();
      this.handleResponse(responseData, requestId, serviceName);
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
  
  private handleResponse(response: any, requestId: string, serviceName: string): void {
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
      
      pendingRequest.resolve(response);
      this.pendingRequests.delete(requestId);
    }
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