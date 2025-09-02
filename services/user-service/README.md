# User Service

A microservice for user management built with Domain Driven Design (DDD) and CQRS patterns in Go. Provides user registration, authentication, and profile management through a TCP binary protocol.

## Features

- User registration with email verification
- JWT-based authentication
- OTP verification system
- Profile management with caching
- Rate limiting and idempotency
- TCP binary protocol interface
- Redis caching for performance
- PostgreSQL with GORM

## Architecture

The service follows DDD principles with clear separation of concerns:

- **Domain**: Business logic and entities
- **Application**: Use cases and commands/queries  
- **Infrastructure**: Database, Redis, email services
- **Interface**: TCP protocol handlers

## Project Structure

```
├── cmd/server/          # Application entry point
├── internal/
│   ├── domain/          # Business logic
│   │   ├── entities/    # Domain entities
│   │   └── repositories/ # Repository interfaces
│   ├── application/     # Use cases
│   │   ├── command/     # Write operations
│   │   ├── query/       # Read operations
│   │   └── services/    # Application services
│   ├── infrastructure/ # External services
│   │   └── db/postgres/ # Database implementation
│   └── interface/tcp/   # TCP protocol handlers
└── .env                 # Environment configuration
```

## Prerequisites

- Go 1.21+
- PostgreSQL database
- Redis (optional, for caching)
- Resend account (for email)

## Setup

1. Clone and install dependencies:
```bash
go mod download
```

2. Configure environment variables:
```bash
cp .env.example .env
```

3. Update `.env` with your configuration:
```env
# Database
DATABASE_URL=postgres://user:password@host:port/dbname?sslmode=require

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# JWT
JWTSECRETKEY=your-secret-key

# Email (Resend)
EMAIL_API_KEY=your-resend-api-key
EMAIL_SENDER=onboarding@resend.dev

# Server
TCP_PORT=3005

# OTP
OTP_EXPIRY=5m
OTP_LENGTH=6
```

4. Run the service:
```bash
cd cmd/server
go run main.go
```

## API Methods

The service uses a custom TCP binary protocol. All methods accept JSON payloads:

### Registration Flow
1. **Send OTP**: Request verification code
```json
{
  "username": "john_doe",
  "email": "john@example.com", 
  "password": "password123"
}
```

2. **Verify OTP**: Complete registration
```json
{
  "email": "john@example.com",
  "otp": "123456"
}
```

### Authentication
**Login**: Authenticate user
```json
{
  "username": "john_doe",
  "password": "password123"
}
```

### Profile Management
**Get Profile**: Retrieve user profile
```json
{
  "userID": "uuid-string"
}
```

## Protocol Details

### Message Format
```
[Magic: 2 bytes][Version: 1 byte][Request ID: 16 bytes][Method Length: 1 byte][Method: variable][Content Length: 4 bytes][Content: variable]
```

### Constants
- Magic Bytes: `0x55 0x57`
- Version: `0x01`
- Methods: `register_user`, `login_user`, `send_otp`, `verify_otp`, `get_profile`, `ping`

### Response Format
```json
{
  "status": "success|error",
  "data": {...},
  "message": "error description"
}
```

## Development

### Database Schema
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP,
    username VARCHAR UNIQUE NOT NULL,
    email VARCHAR UNIQUE NOT NULL,
    password VARCHAR NOT NULL,
    tokens TEXT[],
    is_verified BOOLEAN DEFAULT FALSE
);
```

### Key Features
- **Idempotency**: Prevents duplicate operations
- **Rate Limiting**: 5 requests per 15 minutes for OTP operations
- **Caching**: Redis for tokens, profiles, and OTP codes
- **Graceful Shutdown**: Proper cleanup on termination
- **Connection Pooling**: Optimized database connections

### Testing
```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./internal/domain/entities
go test ./internal/application/services
```

## Security

- Password hashing with bcrypt
- JWT token authentication
- Rate limiting protection
- Input validation
- Soft delete for data retention
- Binary protocol validation

## Performance

- Connection pooling
- Redis caching
- Concurrent token updates
- Optimized database queries
- Configurable timeouts and limits