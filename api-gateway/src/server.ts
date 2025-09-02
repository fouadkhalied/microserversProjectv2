import uWS, { HttpResponse, HttpRequest } from 'uWebSockets.js';
import { registerRoutes } from './routes/user.routes';
import { ServiceClient } from './services/server-client';

const serviceClient = new ServiceClient();

function startServer() {
  const app = uWS.App();

  // Setup HTTP routes
  registerRoutes(app, serviceClient);

  // Start server
  app.listen(3000, (token) => {
    if (token) {
      console.log('🚀 API Gateway listening on http://localhost:3000');
    } else {
      console.error('❌ Failed to start server');
    }
  });
}

startServer();