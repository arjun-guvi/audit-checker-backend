# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/audit-app .

# Runtime stage
FROM alpine:3.22
# CA certificates for MongoDB Atlas, Zoho and SMTP over TLS; tzdata for time zones;
# redis for the job queue (started by docker-entrypoint.sh unless REDIS_HOST is external)
RUN apk add --no-cache ca-certificates tzdata redis \
    && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /out/audit-app /app/audit-app
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod 755 /app/docker-entrypoint.sh

# Listen on all interfaces; Render sets PORT at runtime
ENV GIN_MODE=release \
    HOST=0.0.0.0 \
    PORT=8080
EXPOSE 8080

USER app
ENTRYPOINT ["/app/docker-entrypoint.sh"]
# Default: HTTP server and Redis worker in one container, with the bundled Redis (a single
# Render Web Service). For separate API and worker services, point both at one external Redis
# with REDIS_HOST and pass no argument (API only) or "-worker" (worker only).
CMD ["-with-worker"]
