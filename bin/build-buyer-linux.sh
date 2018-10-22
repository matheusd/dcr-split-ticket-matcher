#!/bin/sh

. ./bin/common.sh

rm -fR dist/release/linux64/split-ticket-buyer
mkdir -p dist/release/linux64/split-ticket-buyer
mkdir -p dist/archives/v$VERSION

echo "Building binaries $VERSION..."

echo "Building CLI buyer (linux64)"
go_build \
    dist/release/linux64/split-ticket-buyer/splitticketbuyer \
    ./cmd/splitticketbuyer

echo "Building GUI buyer (linux64)"
go_build \
    dist/release/linux64/split-ticket-buyer/splitticketbuyergui \
    ./cmd/splitticketbuyergui

cp docs/release-readme.md dist/release/linux64/split-ticket-buyer/README.md

ZIPFILE="splitticketbuyer-linux64-$VERSION.tar.gz"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/linux64 && tar -czf ../../archives/v$VERSION/$ZIPFILE split-ticket-buyer

echo "Built Binaries $VERSION"
