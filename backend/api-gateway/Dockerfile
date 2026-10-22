FROM golang:1.26.10-alpine AS builder

RUN apk --no-cache add make

COPY . /src
WORKDIR /src

RUN GOPROXY=https://goproxy.cn make build

FROM alpine:latest

RUN apk upgrade --no-cache && apk add --no-cache ca-certificates curl


# Copy built binaries
COPY --from=builder /src/bin /app
# Copy Swagger UI static files and OpenAPI spec
COPY --from=builder /src/swagger-ui /app/swagger-ui

WORKDIR /app

EXPOSE 8080
EXPOSE 9000
VOLUME /data/conf

CMD ["./api-gateway", "-conf", "/data/conf"]
