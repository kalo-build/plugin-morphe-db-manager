#!/bin/bash
set -e

# Create dist directory if it doesn't exist
mkdir -p ../dist

GOOS=wasip1 GOARCH=wasm go build -o ../dist/morphe-db-manager-v1.0.0.wasm ../cmd/plugin/main.go

echo "Build successful: dist/morphe-db-manager-v1.0.0.wasm"
