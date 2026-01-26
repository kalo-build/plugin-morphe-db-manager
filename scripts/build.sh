#!/bin/bash
set -e

# Build the WASM plugin
echo "Building WASM plugin..."
mkdir -p dist
GOOS=wasip1 GOARCH=wasm go build -o dist/plugin-morphe-db-manager.wasm ./cmd/plugin

echo "Built: dist/plugin-morphe-db-manager.wasm"

# Build the standalone CLI
echo "Building standalone CLI..."
go build -o dist/dbmanager ./cmd/dbmanager

echo "Built: dist/dbmanager"
echo "Done!"

