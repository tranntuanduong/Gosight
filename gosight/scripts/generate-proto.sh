#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_ROOT"

echo "🔧 Installing protoc plugins..."

# Install protoc plugins for Go
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

echo "📦 Generating Go code for ingestor..."

# Create output directory for Go (ingestor)
mkdir -p ingestor/proto/gosight

# Generate Go code for ingestor
protoc --proto_path=proto \
  --go_out=ingestor/proto --go_opt=paths=source_relative \
  --go-grpc_out=ingestor/proto --go-grpc_opt=paths=source_relative \
  proto/gosight/*.proto

echo "📦 Generating Go code for processor..."

# Create output directory for Go (processor)
mkdir -p processor/proto/gosight

# Generate Go code for processor
protoc --proto_path=proto \
  --go_out=processor/proto --go_opt=paths=source_relative \
  --go-grpc_out=processor/proto --go-grpc_opt=paths=source_relative \
  proto/gosight/*.proto

echo "📦 Generating Go code for api..."

# Create output directory for Go (api)
mkdir -p api/proto/gosight

# Generate Go code for api
protoc --proto_path=proto \
  --go_out=api/proto --go_opt=paths=source_relative \
  --go-grpc_out=api/proto --go-grpc_opt=paths=source_relative \
  proto/gosight/*.proto

echo "📦 Generating TypeScript code..."

# Create output directory for TypeScript
mkdir -p sdk/src/proto

# Generate TypeScript (for SDK) - requires ts-proto or similar
# npx protoc --ts_out=sdk/src/proto \
#   --proto_path=proto \
#   proto/gosight/*.proto

echo "✅ Proto generation complete!"
