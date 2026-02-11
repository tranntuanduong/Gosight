# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY ingestor/go.mod ingestor/go.sum ./ingestor/
RUN cd ingestor && go mod download

# Copy proto generated files
COPY ingestor/proto/ ./ingestor/proto/

# Copy source code
COPY ingestor/ ./ingestor/

# Build
RUN cd ingestor && CGO_ENABLED=0 GOOS=linux go build -o /ingestor ./cmd/ingestor

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /ingestor .
COPY config/ingestor.yaml ./config/

# Create directory for GeoIP database
RUN mkdir -p /data/geoip

EXPOSE 50051 8081

CMD ["./ingestor"]
