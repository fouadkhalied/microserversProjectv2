import { HttpRequest, HttpResponse } from 'uWebSockets.js';
import uWS from 'uWebSockets.js';
import { ServiceClient } from '../services/server-client';

export function registerRoutes(app: ReturnType<typeof uWS.App>, client: ServiceClient) {
  // Health check route
  app.get('/', (res: HttpResponse, req: HttpRequest) => {
    res.writeStatus('200 OK')
      .writeHeader('Content-Type', 'application/json')
      .end(JSON.stringify({ status: 'ok', timestamp: new Date().toISOString() }));
  });

  app.post('/api/users/register', (res, req) => {
    let buffer = ''; // This variable will store the incoming request body
    res.onData((chunk, isLast) => { // `onData` event handler processes chunks of data from the client
      buffer += Buffer.from(chunk).toString(); // Append the received chunk to the `buffer`
      
      // When all data is received (isLast is true)
      if (isLast) {
        try {
          // Parse the buffer content as JSON
          const userData = JSON.parse(buffer);
  
          // Validate user data (e.g., check for missing fields)
          if (!userData.username || !userData.password) {
            res.writeStatus('400 Bad Request') // If validation fails, send 400 status
              .end(JSON.stringify({ error: 'Username and password are required' }));
            return;
          }

          console.log(userData);
          
  
          // Send the parsed data to a service using NATS (a message broker) for registration
          client.sendRequest('user-service', 'register', userData)
            .then(response => {
              res.cork(() => {
                res.writeStatus('201 Created')
                  .writeHeader('Content-Type', 'application/json')
                  .end(JSON.stringify(response));
              });
            })
            .catch(err => {
              console.error('Registration error:', err);
              res.cork(() => {
                res.writeStatus('500 Internal Server Error')
                  .end(JSON.stringify({ error: 'Failed to register user' }));
              });
            });
        } catch (err) { // If JSON parsing fails
          console.error('Invalid request body:', err);
          res.writeStatus('400 Bad Request') // Set 400 Bad Request status
            .end(JSON.stringify({ error: 'Invalid JSON format' })); // Send error message for invalid JSON
        }
      }
    });
  
    // Handle the case when the client aborts the request
    res.onAborted(() => {
      console.log('Client aborted registration request');
    });
  });
  

  // Example: User login
  app.post('/api/users/login', (res, req) => {
    let buffer = '';
    res.onData((chunk, isLast) => {
      buffer += Buffer.from(chunk).toString();
      if (isLast) {
        try {
          const userData = JSON.parse(buffer);
          console.log(userData);
          
          client.sendRequest('user-service', 'login', userData)
            .then(response => {
              res.writeStatus('200 OK')
                .writeHeader('Content-Type', 'application/json')
                .end(JSON.stringify(response));
            })
            .catch(err => {
              console.error('Login error:', err);
              res.writeStatus('401 Unauthorized')
                .end(JSON.stringify({ error: 'Authentication failed' }));
            });
        } catch (err) {
          console.error('Invalid request body:', err);
          res.writeStatus('400 Bad Request')
            .end(JSON.stringify({ error: 'Invalid JSON format' }));
        }
      }
    });

    res.onAborted(() => {
      console.log('Client aborted login request');
    });
  });
}
