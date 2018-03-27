#!/bin/sh

rm -fR dist/release/split-tickets/*
mkdir -p dist/release/split-tickets/win32
mkdir -p dist/release/split-tickets/win64
mkdir -p dist/release/split-tickets/linux64
mkdir -p dist/archives

VERSION=`grep -oP "Version\s+ = \"\K[^\"]+(?=\")" pkg/version.go`

echo "Building binaries $VERSION..."

echo "Building buyer (win32)"
env GOOS=windows GOARCH=386 \
    go build \
    -ldflags "-w -s " \
    -o dist/release/split-tickets/win32/splitticketbuyer \
    cmd/splitticketbuyer/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building buyer (win64)"
env GOOS=windows GOARCH=amd64 \
    go build \
    -ldflags "-w -s " \
    -o dist/release/split-tickets/win64/splitticketbuyer \
    cmd/splitticketbuyer/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building buyer (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/split-tickets/linux64/splitticketbuyer \
    cmd/splitticketbuyer/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building service (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/split-tickets/linux64/dcrstmd \
    cmd/dcrstmd/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

cp samples/*.conf dist/release/split-tickets
cp docs/release-readme.md dist/release/split-tickets/README.md

ZIPFILE="dcr-split-ticket-$VERSION.zip"

rm -f dist/archives/$ZIPFILE

cd dist/release && zip -9 -r ../archives/$ZIPFILE *

echo "Built Binaries $VERSION"
