import uWS, { HttpResponse, HttpRequest } from 'uWebSockets.js';
import { registerRoutes } from './routes/user.routes';
import { ServiceClient } from './services/server-client';

const serviceClient = new ServiceClient();

function startServer() {
  const app = uWS.App();

  registerRoutes(app, serviceClient);

  app.listen(4000, (token) => {
    if (token) {
      console.log('🚀 API Gateway listening on http://localhost:4000');
    } else {
      console.error('❌ Failed to start server');
    }
  });
}

startServer()