# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY processor/go.mod processor/go.sum ./processor/
RUN cd processor && go mod download

# Copy source code
COPY processor/ ./processor/

# Build
RUN cd processor && CGO_ENABLED=0 GOOS=linux go build -o /event-processor ./cmd/event-processor

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /event-processor .
COPY config/processor.docker.yaml ./config/processor.yaml

CMD ["./event-processor"]
