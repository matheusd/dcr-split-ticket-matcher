#!/bin/sh

. ./bin/common.sh

rm -fR dist/release/win64/split-ticket-buyer
mkdir -p dist/release/win64/split-ticket-buyer
mkdir -p dist/archives/v$VERSION

echo "Building binaries $VERSION..."

### win64

echo "Building CLI buyer (win64)"
go_build \
    dist/release/win64/split-ticket-buyer/splitticketbuyer.exe \
    ./cmd/splitticketbuyer

echo "Building GUI buyer (win64)"
go_build \
    dist/release/win64/split-ticket-buyer/splitticketbuyergui.exe \
    ./cmd/splitticketbuyergui

cp docs/release-readme.md dist/release/win64/split-ticket-buyer/README.md
cp samples/win64-dlls/* dist/release/win64/split-ticket-buyer/

ZIPFILE="splitticketbuyer-win64-$VERSION.zip"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/win64 && zip -9 -r ../../archives/v$VERSION/$ZIPFILE split-ticket-buyer

echo "Built win64 binaries $VERSION"
