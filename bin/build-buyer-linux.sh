#!/bin/bash

. ./bin/common.sh

ARCH=`uname -m`
PLAT="linux-$ARCH"

rm -fR dist/release/$PLAT/split-ticket-buyer
mkdir -p dist/release/$PLAT/split-ticket-buyer
mkdir -p dist/archives/v$VERSION

echo "Building binaries $VERSION..."

echo "Building CLI buyer ($PLAT)"
go_build \
    dist/release/$PLAT/split-ticket-buyer/splitticketbuyer \
    ./cmd/splitticketbuyer

echo "Building GUI buyer ($PLAT)"
go_build \
    dist/release/$PLAT/split-ticket-buyer/splitticketbuyergui \
    ./cmd/splitticketbuyergui

cp docs/release-readme.md dist/release/$PLAT/split-ticket-buyer/README.md

ZIPFILE="splitticketbuyer-$PLAT-$VERSION.tar.gz"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/$PLAT && tar -czf ../../archives/v$VERSION/$ZIPFILE split-ticket-buyer

echo "Built Binaries $VERSION"
