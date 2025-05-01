// api-gateway/src/services/service-client.ts

import axios from 'axios';
import path from 'path';
import { connect, NatsConnection, StringCodec } from 'nats';
import grpc from '@grpc/grpc-js';
import protoLoader from '@grpc/proto-loader';

export class ServiceClient {
  private natsConnection: NatsConnection | null = null;
  private sc = StringCodec();
  private userGrpcClient: any;
  private connectionRetries = 0;
  private maxRetries = 5;

  constructor() {
    this.initNatsConnection();
    this.initUserGrpcClient();
  }

  // === NATS INITIALIZATION ===
  private async initNatsConnection() {
    try {
      this.natsConnection = await connect({
        servers: process.env.NATS_URL || 'nats://localhost:4222',
        reconnect: true,
        reconnectTimeWait: 2000,
        maxReconnectAttempts: 10
      });
      console.log('✅ Connected to NATS server');
      this.connectionRetries = 0;

      this.natsConnection.closed().then(() => {
        console.log('NATS connection closed');
        this.natsConnection = null;
        
        // Attempt to reconnect if not shutting down
        if (this.connectionRetries < this.maxRetries) {
          this.connectionRetries++;
          console.log(`Attempting to reconnect to NATS (${this.connectionRetries}/${this.maxRetries})`);
          setTimeout(() => this.initNatsConnection(), 3000);
        }
      });
    } catch (error) {
      console.error('❌ Failed to connect to NATS:', error);
      
      // Retry connection with backoff
      if (this.connectionRetries < this.maxRetries) {
        this.connectionRetries++;
        const delay = Math.min(1000 * Math.pow(2, this.connectionRetries), 30000);
        console.log(`Retrying NATS connection in ${delay}ms (${this.connectionRetries}/${this.maxRetries})`);
        setTimeout(() => this.initNatsConnection(), delay);
      }
    }
  }

  // === gRPC CLIENT INITIALIZATION ===
  private initUserGrpcClient() {
    try {
      const protoPath = path.join(__dirname, '../../proto/user.proto');
      const packageDef = protoLoader.loadSync(protoPath, {
        keepCase: true,
        longs: String,
        enums: String,
        defaults: true,
        oneofs: true,
      });

      const userServiceUrl = process.env.USER_SERVICE_GRPC_URL || 'localhost:50051';
      const grpcObj = grpc.loadPackageDefinition(packageDef) as any;
      
      // Use secure credentials in production
      const credentials = process.env.NODE_ENV === 'production' 
        ? grpc.credentials.createSsl() 
        : grpc.credentials.createInsecure();
        
      this.userGrpcClient = new grpcObj.UserService(
        userServiceUrl,
        credentials
      );
      
      console.log(`✅ Initialized gRPC client for user-service at ${userServiceUrl}`);
    } catch (error) {
      console.error('❌ Failed to initialize gRPC client:', error);
    }
  }

  // === HTTP (GET) ===
  async fetchFromService(serviceName: string, endpoint: string) {
    try {
      const serviceUrl = this.getServiceUrl(serviceName);
      const response = await axios.get(`${serviceUrl}${endpoint}`, {
        timeout: 5000,
        headers: {
          'Content-Type': 'application/json'
        }
      });
      return response.data;
    } catch (error) {
      console.error(`Error fetching from ${serviceName}:`, error);
      throw new Error(`Service communication error with ${serviceName}: ${(error as Error).message}`);
    }
  }

  // === POST TO SERVICE (supports gRPC for user-service) ===
  async postToService(serviceName: string, method: string, payload: any) {
    if (serviceName === 'user-service' && this.userGrpcClient) {
      return this.callGrpcMethod(method, payload);
    }

    // Fallback to HTTP POST
    try {
      const serviceUrl = this.getServiceUrl(serviceName);
      const response = await axios.post(`${serviceUrl}${method}`, payload, {
        timeout: 5000,
        headers: {
          'Content-Type': 'application/json'
        }
      });
      return response.data;
    } catch (error) {
      console.error(`Error posting to ${serviceName} via HTTP:`, error);
      throw new Error(`Service communication error with ${serviceName}: ${(error as Error).message}`);
    }
  }

  // Helper method to handle gRPC calls
  private callGrpcMethod(method: string, payload: any): Promise<any> {
    return new Promise((resolve, reject) => {
      if (!this.userGrpcClient) {
        return reject(new Error('gRPC client not initialized'));
      }

      // Map API endpoint to gRPC method
      let grpcMethod: string;
      switch (method) {
        case '/user/register':
          grpcMethod = 'RegisterUser';
          break;
        case '/user/login':
          grpcMethod = 'LoginUser';
          break;
        case '/user/get':
          grpcMethod = 'GetUser';
          break;
        default:
          return reject(new Error(`Unsupported gRPC method: ${method}`));
      }

      // Call the appropriate gRPC method
      this.userGrpcClient[grpcMethod](payload, (err: any, response: any) => {
        if (err) return reject(new Error(`gRPC call failed: ${err.message}`));
        resolve(response);
      });
    });
  }

  // === SEND MESSAGE TO SERVICE VIA NATS ===
  async sendMessageToService(serviceName: string, subject: string, payload: any) {
    if (!this.natsConnection) {
      try {
        await this.initNatsConnection();
        if (!this.natsConnection) {
          throw new Error('NATS connection not available');
        }
      } catch (error) {
        throw new Error(`NATS connection failed: ${(error as Error).message}`);
      }
    }

    try {
      const data = JSON.stringify(payload);
      const response = await this.natsConnection.request(subject, this.sc.encode(data), {
        timeout: 5000,
      });

      return JSON.parse(this.sc.decode(response.data));
    } catch (error) {
      console.error(`Error sending message to ${serviceName} via NATS:`, error);
      throw new Error(`NATS communication error with ${serviceName}: ${(error as Error).message}`);
    }
  }

  // === SERVICE URL RESOLVER ===
  private getServiceUrl(serviceName: string): string {
    const serviceMap: { [key: string]: string } = {
      'product-service': process.env.PRODUCT_SERVICE_URL || 'http://product-service:8080',
      'user-service': process.env.USER_SERVICE_URL || 'http://user-service:3001',
      'order-service': process.env.ORDER_SERVICE_URL || 'http://order-service:8082',
      'cart-service': process.env.CART_SERVICE_URL || 'http://cart-service:8083',
    };

    const serviceUrl = serviceMap[serviceName];
    if (!serviceUrl) {
      console.warn(`No predefined URL for service: ${serviceName}, using default pattern`);
      return process.env[`${serviceName.toUpperCase().replace(/-/g, '_')}_URL`] || `http://${serviceName}:8080`;
    }

    return serviceUrl;
  }

  // Method to check health of the service client connections
  async checkHealth(): Promise<{ nats: boolean, grpc: boolean }> {
    const health = {
      nats: this.natsConnection !== null,
      grpc: this.userGrpcClient !== null
    };

    return health;
  }
}