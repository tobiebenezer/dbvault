#!/usr/bin/env sh
set -eu
go mod download
go mod verify
CGO_ENABLED=1 go test ./...
