# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/audit-app .

# Runtime stage
FROM alpine:3.22
# CA certificates for MongoDB Atlas, Zoho and SMTP over TLS; tzdata for time zones
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=build /out/audit-app /app/audit-app

# Listen on all interfaces; Render sets PORT at runtime
ENV GIN_MODE=release \
    HOST=0.0.0.0 \
    PORT=8080
EXPOSE 8080

USER app
ENTRYPOINT ["/app/audit-app"]
# Default: HTTP server and Redis worker in one container (a single Render Web Service).
# To run them as separate services instead, override the command:
#   API only:    /app/audit-app
#   worker only: /app/audit-app -worker
CMD ["-with-worker"]
