#!/bin/sh

rm -fR dist/release/linux64/split-ticket-buyer
mkdir -p dist/release/linux64/split-ticket-buyer
mkdir -p dist/archives

VERSION=`grep -oP "Version\s+ = \"\K[^\"]+(?=\")" pkg/version.go`

echo "Building binaries $VERSION..."

echo "Building CLI buyer (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/linux64/split-ticket-buyer/splitticketbuyer \
    cmd/splitticketbuyer/*.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building GUI buyer (linux64)"
env GOOS=linux GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/linux64/split-ticket-buyer/splitticketbuyergui \
    cmd/splitticketbuyergui/*.go
if [[ $? != 0 ]] ; then exit 1 ; fi

cp docs/release-readme.md dist/release/linux64/split-ticket-buyer/README.md

ZIPFILE="splitticketbuyer-linux64-$VERSION.tar.gz"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/linux64 && tar -czf ../../archives/v$VERSION/$ZIPFILE split-ticket-buyer

echo "Built Binaries $VERSION"
