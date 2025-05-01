// api-gateway/src/routes/user.routes.ts

import { HttpRequest, HttpResponse } from 'uWebSockets.js';
import uWS from 'uWebSockets.js';
import { ServiceClient } from '../services/server-client';

export function registerRoutes(app: ReturnType<typeof uWS.App>, client: ServiceClient) {
  // User registration endpoint
  app.post('/api/users/register', (res, req) => {
    let buffer = '';
    
    res.onData((chunk, isLast) => {
      buffer += Buffer.from(chunk).toString();
      
      if (isLast) {
        try {
          const userData = JSON.parse(buffer);
          
          // Basic validation
          if (!userData.username || !userData.password) {
            res.writeStatus('400 Bad Request')
              .writeHeader('Content-Type', 'application/json')
              .end(JSON.stringify({ error: 'Username and password are required' }));
            return;
          }
          
          // Call user service via gRPC
          client.postToService('user-service', '/user/register', userData)
            .then(response => {
              res.writeStatus('201 Created')
                .writeHeader('Content-Type', 'application/json')
                .end(JSON.stringify(response));
            })
            .catch(err => {
              console.error('Registration error:', err);
              res.writeStatus('500 Internal Server Error')
                .writeHeader('Content-Type', 'application/json')
                .end(JSON.stringify({ error: 'Failed to register user' }));
            });
        } catch (err) {
          console.error('Invalid request body:', err);
          res.writeStatus('400 Bad Request')
            .writeHeader('Content-Type', 'application/json')
            .end(JSON.stringify({ error: 'Invalid JSON format' }));
        }
      }
    });
    
    res.onAborted(() => {
      console.log('Client aborted registration request');
    });
  });
  
  // User login endpoint
  app.post('/api/users/login', (res, req) => {
    let buffer = '';
    
    res.onData((chunk, isLast) => {
      buffer += Buffer.from(chunk).toString();
      
      if (isLast) {
        (async () => {
          try {
            const userData = JSON.parse(buffer);
            
            // Basic validation
            if (!userData.username || !userData.password) {
              res.writeStatus('400 Bad Request')
                .writeHeader('Content-Type', 'application/json')
                .end(JSON.stringify({ error: 'Username and password are required' }));
              return;
            }
            
            const response = await client.postToService('user-service', '/user/login', userData);
            
            res.writeStatus('200 OK')
              .writeHeader('Content-Type', 'application/json')
              .end(JSON.stringify(response));
          } catch (err) {
            console.error('Login error:', err);
            res.writeStatus('400 Bad Request')
              .writeHeader('Content-Type', 'application/json')
              .end(JSON.stringify({ error: 'Invalid credentials or format' }));
          }
        })();
      }
    });
    
    res.onAborted(() => {
      console.log('Client aborted login request');
    });
  });

  // Get user profile endpoint
  app.get('/api/users/profile', (res, req) => {
    // Get the authorization header
    const authToken = req.getHeader('authorization');
    if (!authToken) {
      res.writeStatus('401 Unauthorized')
        .writeHeader('Content-Type', 'application/json')
        .end(JSON.stringify({ error: 'Authentication required' }));
      return;
    }

    // Example: extract user ID (this would typically come from validating a JWT)
    const userId = '123'; // Placeholder
    
    client.postToService('user-service', '/user/get', { id: userId })
      .then(userProfile => {
        res.writeStatus('200 OK')
          .writeHeader('Content-Type', 'application/json')
          .end(JSON.stringify(userProfile));
      })
      .catch(err => {
        console.error('Error fetching user profile:', err);
        res.writeStatus('500 Internal Server Error')
          .writeHeader('Content-Type', 'application/json')
          .end(JSON.stringify({ error: 'Failed to fetch user profile' }));
      });
  });

  // Health check endpoint
  app.get('/api/health', (res, req) => {
    res.writeStatus('200 OK')
      .writeHeader('Content-Type', 'application/json')
      .end(JSON.stringify({ status: 'UP' }));
  });
}