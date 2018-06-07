#!/bin/sh

gometalinter \
    --disable-all \
    --vendor \
    --enable=gofmt \
    --enable=vet \
    --enable=gosimple \
    --enable=unconvert \
    --enable=ineffassign \
    --enable=golint \
    --enable=interfacer \
    --enable=deadcode \
    ./...
