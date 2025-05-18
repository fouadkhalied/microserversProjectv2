# API Gateway

A high-performance API Gateway built with TypeScript and uWebSockets.js that handles client communication and routes requests to internal microservices using binary TCP protocol.

## Features

- High-performance HTTP server using uWebSockets.js
- Binary TCP communication with internal services
- JWT-based authentication middleware
- Request/Response streaming
- Error handling and logging
- Health check endpoint

## API Endpoints

### User Service Routes

| Endpoint | Method | Description | Authentication |
|----------|--------|-------------|----------------|
| `/` | GET | Health check endpoint | No |
| `/api/users/register` | POST | Register new user | No |
| `/api/users/verify` | POST | Verify user email with OTP | No |
| `/api/users/login` | POST | User login | No |
| `/api/users/profile` | GET | Get user profile | Yes |

### Request/Response Format

#### Register User
```json
// Request
POST /api/users/register
{
  "username": "string",
  "password": "string"
}

// Response
{
  "status": "success",
  "message": "User registered successfully"
}
```

#### Verify User
```json
// Request
POST /api/users/verify
{
  "email": "string",
  "otp": "string"
}

// Response
{
  "status": "success",
  "message": "User verified successfully"
}
```

#### Login
```json
// Request
POST /api/users/login
{
  "username": "string",
  "password": "string"
}

// Response
{
  "token": "string",
  "user": {
    "id": "string",
    "username": "string"
  }
}
```

#### Get Profile
```json
// Request
GET /api/users/profile
Authorization: Bearer <token>

// Response
{
  "user": {
    "id": "string",
    "username": "string",
    "email": "string"
  }
}
```

## Error Responses

All endpoints return error responses in the following format:
```json
{
  "error": "Error message"
}
```

Common HTTP status codes:
- 200: Success
- 201: Created
- 400: Bad Request
- 401: Unauthorized
- 500: Internal Server Error

## Development

### Prerequisites

- Node.js 16+
- TypeScript
- Docker (optional)

### Installation

```bash
npm install
```

### Running Locally

```bash
npm run dev
```

### Building

```bash
npm run build
```

### Running with Docker

```bash
docker build -t api-gateway .
docker run -p 3000:3000 api-gateway
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | 3000 |
| `JWT_SECRET` | Secret key for JWT | required |
| `USER_SERVICE_HOST` | User service host | localhost |
| `USER_SERVICE_PORT` | User service port | 3001 | 