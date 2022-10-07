#!/bin/bash
GOOS=linux GOARCH=amd64 go build -o bin/remnotetologseq-amd64-linux main.go
GOOS=linux GOARCH=arm64 go build -o bin/remnotetologseq-arm64-linux main.go
GOOS=darwin GOARCH=amd64 go build -o bin/remnotetologseq-amd64-darwin main.go
GOOS=darwin GOARCH=arm64 go build -o bin/remnotetologseq-arm64-darwin main.go
GOOS=windows GOARCH=amd64 go build -o bin/remnotetologseq-amd64-windows.exe main.go
GOOS=windows GOARCH=arm64 go build -o bin/remnotetologseq-arm64-windows.exe main.go
