#!/bin/bash

set -e

echo "🔧 Installing protoc plugins..."

# Install protoc plugins for Go
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

echo "📦 Generating Go code..."

# Generate Go code
protoc --proto_path=proto \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/gosight/*.proto

echo "📦 Generating TypeScript code..."

# Create output directory for TypeScript
mkdir -p sdk/src/proto

# Generate TypeScript (for SDK)
npx protoc --ts_out=sdk/src/proto \
  --proto_path=proto \
  proto/gosight/*.proto

echo "✅ Proto generation complete!"
