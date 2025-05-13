import { HttpResponse, HttpRequest } from 'uWebSockets.js';
import jwt from 'jsonwebtoken';

export const authMiddleware = (res: HttpResponse, req: HttpRequest): any | null => {
  try {
    const token = req.getHeader('authorization')?.split(' ')[1];
    
    console.log(token);

    if (!token) {
      res.writeStatus('401 Unauthorized');
      res.end(JSON.stringify({ message: 'no token found' }));
      return null;
    }

    const decoded = jwt.verify(token, process.env.JWT_SECRET || 'fouad');
    return decoded;
  } catch (error) {
    res.writeStatus('401 Unauthorized');
    res.end(JSON.stringify({ message: 'unverified token' }));
    return null;
  }
};
