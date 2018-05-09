#!/bin/sh

rm -fR dist/release/win64/split-ticket-buyer
mkdir -p dist/release/win64/split-ticket-buyer
mkdir -p dist/archives

VERSION=`grep -oP "Version\s+ = \"\K[^\"]+(?=\")" pkg/version.go`

echo "Building binaries $VERSION..."

### win64

echo "Building CLI buyer (win64)"
env GOOS=windows GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/win64/split-ticket-buyer/splitticketbuyer.exe \
    cmd/splitticketbuyer/*.go
if [[ $? != 0 ]] ; then exit 1 ; fi

echo "Building GUI buyer (win64)"
env GOOS=windows GOARCH=amd64 \
    go build \
    -v \
    -o dist/release/win64/split-ticket-buyer/splitticketbuyergui.exe \
    cmd/splitticketbuyergui/*.go
if [[ $? != 0 ]] ; then exit 1 ; fi

cp docs/release-readme.md dist/release/win64/split-ticket-buyer/README.md
cp samples/win64-dlls/* dist/release/win64/split-ticket-buyer/

ZIPFILE="splitticketbuyer-win64-$VERSION.zip"

rm -f dist/archives/$ZIPFILE

cd dist/release/win64 && zip -9 -r ../../archives/$ZIPFILE split-ticket-buyer

echo "Built win64 binaries $VERSION"
