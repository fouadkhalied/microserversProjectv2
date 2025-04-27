# User Service

A microservice for user authentication and registration built with Go, PostgreSQL, Redis, and NATS messaging.

## Features

- User registration with secure password hashing
- User authentication with JWT tokens
- Dual communication channels via REST API and NATS messaging
- Token caching with Redis
- Clean architecture implementation

## Tech Stack

- **Language**: Go
- **Database**: PostgreSQL
- **Cache**: Redis
- **Messaging**: NATS
- **Authentication**: JWT

## Prerequisites

- Go 1.20+
- PostgreSQL
- Redis
- NATS Server

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/your-org/user-service.git
   cd user-service
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Set environment variables:
   ```bash
   export JWTSECRETKEY=your_jwt_secret_key
   export PostgreSQL=postgresql://username:password@host:port/dbname
   ```

## Running the Service

Start the service:
```bash
go run cmd/main.go
```

The service will start on port 3001.

## API Endpoints

### Register User
```
POST /user/register
Content-Type: application/json

{
  "username": "example",
  "email": "user@example.com",
  "password": "securepassword"
}
```

### Login User
```
POST /user/login
Content-Type: application/json

{
  "username": "example",
  "password": "securepassword"
}
```

Response:
```json
{
  "token": "jwt_token_here"
}
```

## NATS Messages

The service subscribes to:

- `user.register` - Register a new user
- `user.login` - Authenticate a user

## Architecture

The service follows clean architecture principles:
- **Domain**: Core business entities
- **Repository**: Data access layer
- **Usecase**: Business logic
- **Delivery**: HTTP and messaging handlers

## Configuration

Configuration is loaded from environment variables:
- `JWTSECRETKEY`: Secret key for JWT signing
- `PostgreSQL`: PostgreSQL connection string

## Development

### Project Structure
```
├── cmd/
│   └── main.go                    # Application entry point
├── internal/
│   ├── config/                    # Configuration loading
│   ├── delivery/                  # HTTP and messaging adapters
│   │   ├── handler/               # HTTP handlers
│   │   └── messaging/             # NATS messaging
│   ├── domain/                    # Core domain models
│   ├── infrastructure/            # Supporting services
│   ├── repository/                # Data access layer
│   └── usecase/                   # Business logic
└── README.md                      # This file
```