#!/bin/sh

mkdir -p dist/release/win32
mkdir -p dist/release/win64
mkdir -p dist/release/linux64
mkdir -p dist/archives

VERSION=`grep -oP "Version\s+ = \"\K[^\"]+(?=\")" pkg/version.go`

echo "Building binaries $VERSION..."

echo "Building buyer (win32)"
env GOOS=windows GOARCH=386 \
    go build \
    -ldflags "-w -s " \
    -o dist/release/win32/splitticketbuyer \
    cmd/splitticketbuyer/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building buyer (win64)"
env GOOS=windows GOARCH=amd64 \
    go build \
    -ldflags "-w -s " \
    -o dist/release/win64/splitticketbuyer \
    cmd/splitticketbuyer/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building buyer (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/linux64/splitticketbuyer \
    cmd/splitticketbuyer/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building service (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/linux64/dcrstmd \
    cmd/dcrstmd/main.go
if [[ $? != 0 ]] ; then exit 1 ; fi

cp samples/*.conf dist/release
cp docs/release-readme.md dist/release/README.md

ZIPFILE="dcr-split-ticket-$VERSION.zip"

rm -f dist/archives/$ZIPFILE

cd dist/release && zip -9 -r ../archives/$ZIPFILE *

echo "Built Binaries $VERSION"
