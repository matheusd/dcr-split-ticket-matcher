#!/bin/sh

. ./bin/common.sh

rm -fR dist/release/linux64/dcrstmd
mkdir -p dist/release/linux64/dcrstmd
mkdir -p dist/archives/v$VERSION

echo "Building service binaries $VERSION..."

echo "Building service (linux64)"
go_build \
    dist/release/linux64/dcrstmd/dcrstmd \
    ./cmd/dcrstmd

cp samples/dcrstmd.conf dist/release/linux64/dcrstmd
cp docs/release-readme.md dist/release/linux64/dcrstmd/README.md

ZIPFILE="dcrstmd-linux64-$VERSION.tar.gz"

rm -f dist/archives/v$VERSION/$ZIPFILE

cd dist/release/linux64 && tar -czf ../../archives/v$VERSION/$ZIPFILE dcrstmd

echo "Built service binaries $VERSION"
