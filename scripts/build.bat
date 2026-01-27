@echo off
set GOOS=wasip1
set GOARCH=wasm

REM Create dist directory if it doesn't exist
if not exist "..\dist" mkdir "..\dist"

go build -o ../dist/morphe-db-manager-v1.0.0.wasm ../cmd/plugin/main.go

if %errorlevel% equ 0 (
    echo Build successful: dist/morphe-db-manager-v1.0.0.wasm
) else (
    echo Build failed
    exit /b 1
)