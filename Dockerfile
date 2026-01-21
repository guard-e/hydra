FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
# gcc and musl-dev are required for CGO (go-sqlite3)
RUN apk add --no-cache gcc musl-dev

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -o hydra-server cmd/hydra/main.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Install necessary runtime dependencies
RUN apk add --no-cache ca-certificates sqlite

# Copy binary from builder
COPY --from=builder /app/hydra-server .

# Copy web assets
COPY --from=builder /app/web ./web

# Create directories for data
RUN mkdir -p voice_storage

# Expose ports
# 8081: Web Interface / HTTP API
# 8080: P2P Mesh Transport
EXPOSE 8081 8080

# Command to run
CMD ["./hydra-server"]
