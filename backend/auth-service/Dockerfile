# Build stage
FROM golang:1.26.9-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o auth-service ./cmd/auth-service

# Final stage
FROM alpine:latest

RUN apk upgrade --no-cache && apk add --no-cache ca-certificates curl

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/auth-service .

# Expose port
EXPOSE 8081

# Run the application
CMD ["./auth-service"] 