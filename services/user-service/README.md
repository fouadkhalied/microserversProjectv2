# User Service

A microservice for user authentication and registration built with Go, operating as a TCP server that receives binary messages from the API Gateway.

## Features

- Binary TCP message handling
- User registration with secure password hashing
- Email OTP verification system
- JWT-based authentication
- Token caching with Redis
- Clean architecture implementation

## Tech Stack

- **Language**: Go
- **Database**: PostgreSQL
- **Cache**: Redis
- **Authentication**: JWT
- **Email Service**: SMTP

## Prerequisites

- Go 1.20+
- PostgreSQL
- Redis
- SMTP Server

## Running the Service

Start the service:
```bash
go run cmd/main.go
```

The service will start on port 3001.

## Communication Protocol

### Message Format

The service receives binary messages from the API Gateway in the following format:

```
[Message Length: 4 bytes][Message Type: 1 byte][Message Data: variable]
```

### Message Types

1. Register User (0x01)
2. Login (0x02)
3. Verify Token (0x03)
4. Update User (0x04)
5. Delete User (0x05)

## Internal Architecture

The service follows clean architecture principles with the following components:

### Core Components

1. **TCP Server**
   - Handles incoming TCP connections
   - Manages connection pools
   - Implements binary message protocol

2. **Message Handler**
   - Decodes binary messages
   - Routes messages to appropriate handlers
   - Manages message validation

3. **User Service**
   - Implements user-related business logic
   - Handles user registration and authentication
   - Manages user data operations
   - Email verification workflow

4. **Token Service**
   - JWT token generation and validation
   - Token caching with Redis
   - Token refresh mechanism

5. **OTP Service**
   - Generates secure OTP codes
   - Manages OTP expiration
   - Handles OTP verification
   - Email delivery through SMTP

### Data Flow

1. TCP connection established
2. Binary message received and decoded
3. Message routed to appropriate handler
4. Business logic executed
5. For registration:
   - User data validated
   - OTP generated and sent via email
   - User status set to pending verification
6. For OTP verification:
   - OTP validated
   - User status updated to verified
   - JWT token generated
7. Response encoded and sent back

## Project Structure
```
├── cmd/
│   └── main.go                    # Application entry point
├── internal/
│   ├── config/                    # Configuration loading
│   ├── delivery/                  # TCP server implementation
│   │   └── tcp/                   # TCP handlers and protocol
│   ├── domain/                    # Core domain models
│   ├── infrastructure/            # Supporting services
│   ├── repository/                # Data access layer
│   └── usecase/                   # Business logic
└── README.md                      # This file
```
## Configuration

Configuration is loaded from environment variables:
- `JWTSECRETKEY`: Secret key for JWT signing
- `PostgreSQL`: PostgreSQL connection string
- `REDIS_URL`: Redis connection URL
- `TCP_PORT`: TCP server port (default: 3001)
- `SMTP_HOST`: SMTP server host
- `SMTP_PORT`: SMTP server port
- `OTP_EXPIRY`: OTP expiration time in minutes (15)
