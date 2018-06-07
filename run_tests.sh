#!/bin/sh

gometalinter \
    --disable-all --vendor --deadline=10m \
    --enable=gofmt \
    --enable=vet \
    --enable=gosimple \
    --enable=unconvert \
    --enable=ineffassign \
    --enable=golint \
    --enable=interfacer \
    --enable=deadcode \
    --enable=vetshadow \
    --enable=structcheck \
    --enable=goconst \
    ./...
