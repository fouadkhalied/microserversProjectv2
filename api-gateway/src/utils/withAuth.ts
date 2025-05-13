import { authMiddleware } from '../middleware/auth.middleware';
import { HttpResponse, HttpRequest } from 'uWebSockets.js';


function withAuth(handler: (res: HttpResponse, req: HttpRequest, user: any) => void) {
    return (res: HttpResponse, req: HttpRequest) => {
      const user = authMiddleware(res, req);
      if (!user) return; // authMiddleware already ends the response
      handler(res, req, user);
    };
}

export default withAuth