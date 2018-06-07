#!/bin/sh

gometalinter \
    --disable-all --vendor --deadline=10m \
    --enable=gofmt \
    --enable=vet \
    --enable=unconvert \
    --enable=ineffassign \
    --enable=golint \
    --enable=interfacer \
    --enable=deadcode \
    --enable=vetshadow \
    --enable=structcheck \
    --enable=goconst \
    --enable=megacheck \
    --enable=varcheck \
    --enable=gas \
    ./...

if [ $? != 0 ]; then
    echo 'gometalinter failed'
    exit 1
fi

go test ./...
