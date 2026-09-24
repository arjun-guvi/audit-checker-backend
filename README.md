# Audit Checker Backend

Backend service for audit checking system.

## Features

- SAP audit management
- Email notifications
- Background job processing
- JWT-based authentication

## Prerequisites

- Go 1.19+
- MongoDB
- Redis

## Installation

1. Clone the repository
2. Install dependencies:
   ```
   go mod tidy
   ```
3. Set up environment variables in `.env` file
4. Run the application:
   ```
   go run main.go
   ```

## Authentication

This application uses JWT for authentication.

### Register
```
POST /register
{
  "username": "user123",
  "email": "user@example.com",
  "password": "password123"
}
```

### Login
```
POST /login
{
  "username": "user123",
  "password": "password123"
}
```

Returns a JWT token that should be included in the Authorization header for protected routes:
```
Authorization: Bearer <token>
```

### Get Current User
```
GET /me
Authorization: Bearer <token>
```

## SAP Management

All SAP-related endpoints require authentication.

### List SAP Audits
```
GET /sap/list
```

### Get SAP Audit by ID
```
GET /sap/{id}
```

### Create SAP Audit
```
POST /sap/create
```

### Edit SAP Audit
```
PUT /sap/edit/{id}
```

### Delete SAP Audit
```
DELETE /sap/delete/{id}
```

## Environment Variables

- `PORT`: Server port (default: 8080)
- `MONGO_URI`: MongoDB connection string
- `MONGO_DATABASE`: MongoDB database name
- `REDIS_HOST`: Redis server address
- `REDIS_PASSWORD`: Redis password
- `JWT_SECRET`: Secret key for JWT token signing

## Running in Different Modes

### Web Server Mode (default)
```
go run main.go
```

### Worker Mode
Processes background jobs from Redis:
```
go run main.go -worker
```

### SAP Reminder Worker
Checks for reminders every 30 minutes:
```
go run main.go -sap-worker
```