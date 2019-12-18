#!/usr/bin/env bash

set -ex

go build ./...

golangci-lint run --disable-all --deadline=10m \
    --enable=gofmt \
    --enable=vet \
    --enable=ineffassign \
    --enable=golint \
    --enable=deadcode \
    --enable=vetshadow \
    --enable=goconst

env GORACE=halt_on_error=1 go test ./...
