@echo off
setlocal

REM Build the WASM plugin
echo Building WASM plugin...
if not exist dist mkdir dist
set GOOS=wasip1
set GOARCH=wasm
go build -o dist\plugin-morphe-db-manager.wasm .\cmd\plugin

echo Built: dist\plugin-morphe-db-manager.wasm

REM Build the standalone CLI
echo Building standalone CLI...
set GOOS=
set GOARCH=
go build -o dist\dbmanager.exe .\cmd\dbmanager

echo Built: dist\dbmanager.exe
echo Done!

