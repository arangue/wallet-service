# Stage 1: Build the binary
FROM golang:1.26.1-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /src

# Leverage Docker cache for dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code (targeted to preserve layer cache on non-Go file changes)
COPY cmd/ cmd/
COPY internal/ internal/

# Build the application with optimizations
# -ldflags="-w -s" removes debug info to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/server ./cmd/server

# Stage 2: Final lightweight image
FROM alpine:3.21

# Add a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Install CA certificates for secure connections
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /bin/server .

# Copy migrations (required for the app to run them on startup)
COPY --from=builder /src/internal/infrastructure/postgres/migrations ./internal/infrastructure/postgres/migrations

# Use the non-root user
USER appuser

# Document the port
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["./server"]
