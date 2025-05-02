// api-gateway/src/services/service-client.ts
import axios from 'axios';
import { connect, NatsConnection, StringCodec } from 'nats';
import WebSocket from 'uWebSockets.js';
import { WebSocket as WS } from 'ws';
import { v4 as uuidv4 } from 'uuid';

// Binary message structure:
// [
//   Header (2 bytes): 0x55, 0x57 (UW magic bytes)
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
}

export class ServiceClient {
  private natsConnection: NatsConnection | null = null;
  private sc = StringCodec();
  
  // WebSocket connections to internal services
  private wsConnections: Map<string, WS> = new Map();
  private pendingRequests: Map<string, PendingRequest> = new Map();
  private connectionPromises: Map<string, Promise<WS>> = new Map();
  
  // Constants for the binary protocol
  private readonly MAGIC_BYTES = new Uint8Array([0x55, 0x57]); // "UW"
  private readonly REQUEST_TIMEOUT = 5000; // 5 seconds
  
  constructor() {
    this.initNatsConnection(); // Keep NATS for service-to-service communication
  }
  
  private async initNatsConnection() {
    try {
      this.natsConnection = await connect({
        servers: process.env.NATS_URL || 'http://localhost:4222'
      });
      console.log('✅ Connected to NATS server');
      
      // Setup a handler when connection is closed
      this.natsConnection.closed().then(() => {
        console.log('NATS connection closed');
        this.natsConnection = null;
      });
    } catch (error) {
      console.error('❌ Failed to connect to NATS:', error);
    }
  }
  
  // Connect to a service using uWebSockets
  private async connectToService(serviceName: string): Promise<WS> {
    // If there's already a connection attempt in progress, return that promise
    if (this.connectionPromises.has(serviceName)) {
      return this.connectionPromises.get(serviceName)!;
    }
    
    // If there's an existing connection, return it
    if (this.wsConnections.has(serviceName) && 
        (this.wsConnections.get(serviceName) as any).readyState === 1) {
      return this.wsConnections.get(serviceName)!;
    }
    
    // Create new connection promise
    const connectionPromise = new Promise<WS>((resolve, reject) => {
      const serviceUrl = this.getServiceWsUrl(serviceName);
      console.log(`Connecting to ${serviceName} at ${serviceUrl}...`);
      
      // Create WebSocket connection
      const ws = new WS(serviceUrl);
      
      // Set up event handlers
      ws.on('open', () => {
        console.log(`✅ Connected to ${serviceName} via WebSocket`);
        this.wsConnections.set(serviceName, ws);
        this.connectionPromises.delete(serviceName);
        resolve(ws);
      });
      
      ws.on('message', (data: ArrayBuffer) => {
        this.handleServiceMessage(data);
      });
      
      ws.on('close', () => {
        console.log(`WebSocket connection to ${serviceName} closed`);
        this.wsConnections.delete(serviceName);
        
        // Reject all pending requests for this service
        for (const [requestId, request] of this.pendingRequests.entries()) {
          request.reject(new Error(`Connection to ${serviceName} closed`));
          clearTimeout(request.timer);
          this.pendingRequests.delete(requestId);
        }
      });
      
      ws.on('error', (error : any) => {
        console.error(`WebSocket error with ${serviceName}:`, error);
        reject(error);
      });
    });
    
    this.connectionPromises.set(serviceName, connectionPromise);
    return connectionPromise;
  }
  
  // Handle binary message from service
  private handleServiceMessage(data: ArrayBuffer) {
    try {
      const buffer = new Uint8Array(data);
      
      // Check magic bytes
      if (buffer[0] !== this.MAGIC_BYTES[0] || buffer[1] !== this.MAGIC_BYTES[1]) {
        console.error('Invalid message format: incorrect magic bytes');
        return;
      }
      
      // Extract request ID (16 bytes)
      const requestIdBytes = buffer.slice(2, 18);
      const requestId = this.bytesToUUID(requestIdBytes);
      
      // Find pending request
      const pendingRequest = this.pendingRequests.get(requestId);
      if (!pendingRequest) {
        console.warn(`Received response for unknown request ID: ${requestId}`);
        return;
      }
      
      // Extract content length (4 bytes)
      const contentLength = new DataView(buffer.buffer).getUint32(19, true);
      
      // Extract content
      const content = buffer.slice(23, 23 + contentLength);
      const jsonContent = new TextDecoder().decode(content);
      
      // Resolve the pending request
      try {
        const parsedContent = JSON.parse(jsonContent);
        pendingRequest.resolve(parsedContent);
      } catch (error) {
        pendingRequest.reject(new Error('Invalid JSON response'));
      }
      
      // Clean up
      clearTimeout(pendingRequest.timer);
      this.pendingRequests.delete(requestId);
      
    } catch (error) {
      console.error('Error processing service message:', error);
    }
  }
  
  // Convert UUID string to bytes
  private uuidToBytes(uuid: string): Uint8Array {
    const bytes = new Uint8Array(16);
    const parts = uuid.replace(/-/g, '').match(/.{2}/g) || [];
    
    for (let i = 0; i < 16; i++) {
      bytes[i] = parseInt(parts[i], 16);
    }
    
    return bytes;
  }
  
  // Convert bytes to UUID string
  private bytesToUUID(bytes: Uint8Array): string {
    const hexBytes = Array.from(bytes).map(b => b.toString(16).padStart(2, '0'));
    const uuid = [
      hexBytes.slice(0, 4).join(''),
      hexBytes.slice(4, 6).join(''),
      hexBytes.slice(6, 8).join(''),
      hexBytes.slice(8, 10).join(''),
      hexBytes.slice(10, 16).join('')
    ].join('-');
    
    return uuid;
  }
  
  // Send a binary request to a service
  async sendBinaryRequest(serviceName: string, method: string, payload: any): Promise<any> {
    try {
      // Connect to the service if not already connected
      const ws = await this.connectToService(serviceName);
      
      // Generate request ID
      const requestId = uuidv4();
      const requestIdBytes = this.uuidToBytes(requestId);
      
      // Encode method
      const methodBytes = new TextEncoder().encode(method);
      const methodLength = methodBytes.length;
      
      // Encode payload
      const payloadBytes = new TextEncoder().encode(JSON.stringify(payload));
      const payloadLength = payloadBytes.length;
      
      // Calculate total message length
      const messageLength = 2 + 16 + 1 + methodLength + 4 + payloadLength;
      
      // Create message buffer
      const message = new Uint8Array(messageLength);
      let offset = 0;
      
      // Add magic bytes
      message.set(this.MAGIC_BYTES, offset);
      offset += 2;
      
      // Add request ID
      message.set(requestIdBytes, offset);
      offset += 16;
      
      // Add method length
      message[offset] = methodLength;
      offset += 1;
      
      // Add method
      message.set(methodBytes, offset);
      offset += methodLength;
      
      // Add payload length
      new DataView(message.buffer).setUint32(offset, payloadLength, true);
      offset += 4;
      
      // Add payload
      message.set(payloadBytes, offset);
      
      // Create promise for the response
      const responsePromise = new Promise<any>((resolve, reject) => {
        // Set timeout
        const timer = setTimeout(() => {
          this.pendingRequests.delete(requestId);
          reject(new Error(`Request to ${serviceName} timed out`));
        }, this.REQUEST_TIMEOUT);
        
        // Store the pending request
        this.pendingRequests.set(requestId, { resolve, reject, timer });
      });
      
      // Send the message
      ws.send(message);
      
      // Wait for the response
      return await responsePromise;
      
    } catch (error) {
      console.error(`Error sending binary request to ${serviceName}:`, error);
      throw error;
    }
  }
  
  // Fetch data from a service using HTTP
  async fetchFromService(serviceName: string, endpoint: string) {
    try {
      const serviceUrl = this.getServiceUrl(serviceName);
      const response = await axios.get(`${serviceUrl}${endpoint}`);
      return response.data;
    } catch (error) {
      console.error(`Error fetching from ${serviceName}:`, error);
      throw error;
    }
  }
  
  // Send a message to a service using NATS (for service-to-service communication)
  // async sendMessageToService(serviceName: string, subject: string, payload: any) {
  //   if (!this.natsConnection) {
  //     try {
  //       await this.initNatsConnection();
  //     } catch (error) {
  //       throw new Error('NATS connection not available');
  //     }
  //   }
    
  //   try {
  //     // Create a request message
  //     const data = JSON.stringify(payload);
      
  //     // Publish the message to NATS with request-reply pattern
  //     const response = await this.natsConnection!.request(
  //       subject,
  //       this.sc.encode(data),
  //       { timeout: 5000 } // 5 second timeout
  //     );
      
  //     // Return the parsed response
  //     return JSON.parse(this.sc.decode(response.data));
  //   } catch (error) {
  //     console.error(`Error sending message to ${serviceName}:`, error);
  //     throw error;
  //   }
  // }
  
  // Helper method to get service URL from environment or service discovery
  private getServiceUrl(serviceName: string): string {
    // In a real implementation, you might use service discovery
    // For now, we'll use environment variables or defaults
    const serviceMap: { [key: string]: string } = {
      'product-service': process.env.PRODUCT_SERVICE_URL || 'http://product-service:8080',
      'user-service': process.env.USER_SERVICE_URL || 'http://localhost:3001',
      'order-service': process.env.ORDER_SERVICE_URL || 'http://order-service:8082',
      'cart-service': process.env.CART_SERVICE_URL || 'http://cart-service:8083',
    };
    
    return serviceMap[serviceName] || `http://${serviceName}:8080`;
  }
  
  // Helper method to get WebSocket service URL
  private getServiceWsUrl(serviceName: string): string {
    const serviceMap: { [key: string]: string } = {
      'product-service': process.env.PRODUCT_SERVICE_WS_URL || 'ws://product-service:9080',
      'user-service': process.env.USER_SERVICE_WS_URL || 'ws://user-service:9081',
      'order-service': process.env.ORDER_SERVICE_WS_URL || 'ws://order-service:9082',
      'cart-service': process.env.CART_SERVICE_WS_URL || 'ws://cart-service:9083',
    };
    
    return serviceMap[serviceName] || `ws://${serviceName}:9080`;
  }
}