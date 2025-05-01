// api-gateway/src/services/service-client.ts
import axios from 'axios';
import { connect, NatsConnection, StringCodec } from 'nats';

export class ServiceClient {
  private natsConnection: NatsConnection | null = null;
  private sc = StringCodec();
  
  constructor() {
    this.initNatsConnection();
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
  
  // Send a message to a service using NATS
  async sendMessageToService(serviceName: string, subject: string, payload: any) {
    if (!this.natsConnection) {
      try {
        await this.initNatsConnection();
      } catch (error) {
        throw new Error('NATS connection not available');
      }
    }
    
    try {
      // Create a request message
      const data = JSON.stringify(payload);
      
      // Publish the message to NATS with request-reply pattern
      const response = await this.natsConnection!.request(
        subject,
        this.sc.encode(data),
        { timeout: 5000 } // 5 second timeout
      );
      
      // Return the parsed response
      return JSON.parse(this.sc.decode(response.data));
    } catch (error) {
      console.error(`Error sending message to ${serviceName}:`, error);
      throw error;
    }
  }
  
  // Helper method to get service URL from environment or service discovery
  private getServiceUrl(serviceName: string): string {
    // In a real implementation, you might use service discovery
    // For now, we'll use environment variables or defaults
    const serviceMap: { [key: string]: string } = {
      'product-service': process.env.PRODUCT_SERVICE_URL || 'http://product-service:8080',
      'user-service': process.env.USER_SERVICE_URL || 'http://user-service:8081',
      'order-service': process.env.ORDER_SERVICE_URL || 'http://order-service:8082',
      'cart-service': process.env.CART_SERVICE_URL || 'http://cart-service:8083',
    };
    
    return serviceMap[serviceName] || `http://${serviceName}:8080`;
  }
}
