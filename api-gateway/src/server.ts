// api-gateway/src/server.ts

import uWS from 'uWebSockets.js';
import { ServiceClient } from './services/server-client';
import { registerRoutes } from './routes/user.routes';

// Initialize service client
const serviceClient = new ServiceClient();

function startServer() {
  const app = uWS.App();
  
  // CORS middleware for preflight requests
  app.options('/*', (res, req) => {
    res.writeStatus('204 No Content')
      .writeHeader('Access-Control-Allow-Origin', '*')
      .writeHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, DELETE, OPTIONS')
      .writeHeader('Access-Control-Allow-Headers', 'content-type, authorization')
      .writeHeader('Access-Control-Max-Age', '86400')
      .end();
  });
  
  // Register all route handlers
  registerRoutes(app, serviceClient);
  
  // Future route registrations (when implemented)
  // registerProductRoutes(app, serviceClient);
  // registerOrderRoutes(app, serviceClient);
  // registerCartRoutes(app, serviceClient);
  
  // 404 handler for undefined routes
  app.any('/*', (res, req) => {
    res.writeStatus('404 Not Found')
      .writeHeader('Content-Type', 'application/json')
      .end(JSON.stringify({ error: 'Endpoint not found' }));
  });
  
  // Start the server
  app.listen("0.0.0.0", 3000, (token) => {
    if (token) {
      console.log('🚀 API Gateway listening on http://localhost:3000');
    } else {
      console.error('❌ Failed to start server');
      process.exit(1);
    }
  });
}

// Start the server
startServer();