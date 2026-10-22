FROM golang:1.26.10-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o notification-service ./cmd/notification-service

FROM alpine:latest
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates curl
WORKDIR /root/
COPY --from=builder /app/notification-service .
EXPOSE 8083
CMD ["./notification-service"]